package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	taskIdempotencyHeader          = "Idempotency-Key"
	taskIdempotencyTTL             = 24 * time.Hour
	maxIdempotencyKeyLen           = 255
	taskIdempotencyLeaseContextKey = "task_idempotency_lease"
)

type taskIdempotencyEntry struct {
	value     string
	expiresAt time.Time
}

var taskIdempotencyStore = struct {
	sync.Mutex
	entries map[string]taskIdempotencyEntry
}{entries: make(map[string]taskIdempotencyEntry)}

type taskIdempotencyLease struct {
	key       string
	token     string
	redis     bool
	committed bool
}

// beginTaskIdempotency claims a task creation key before channel selection and
// billing. A completed key replays the durable task; an in-flight key returns a
// deterministic conflict. Redis is used when configured, with a process-local
// fallback for development and single-node deployments.
func beginTaskIdempotency(c *gin.Context, info *relaycommon.RelayInfo) (*model.Task, *taskIdempotencyLease, *dto.TaskError) {
	if c == nil || info == nil {
		return nil, nil, nil
	}
	if info.TaskRelayInfo == nil {
		return nil, nil, nil
	}
	rawKey := strings.TrimSpace(c.GetHeader(taskIdempotencyHeader))
	if rawKey == "" {
		return nil, nil, nil
	}
	if len(rawKey) > maxIdempotencyKeyLen {
		return nil, nil, service.TaskErrorWrapperLocal(errors.New("Idempotency-Key must be 255 characters or fewer"), "invalid_idempotency_key", 400)
	}
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	info.TaskRelayInfo.IdempotencyKey = rawKey
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", info.UserId, path, rawKey)))
	key := "task:idempotency:" + hex.EncodeToString(digest[:])
	token := "inflight:" + common.NewRequestId()

	if common.RedisEnabled && common.RDB != nil {
		claimed, err := common.RDB.SetNX(context.Background(), key, token, taskIdempotencyTTL).Result()
		if err == nil {
			if claimed {
				return nil, &taskIdempotencyLease{key: key, token: token, redis: true}, nil
			}
			if existing, done := resolveRedisIdempotency(key, info.UserId); done {
				return existing, nil, nil
			}
			return nil, nil, idempotencyInProgressError()
		}
		// Redis outages must not make task submission unavailable. Fall back to
		// the local atomic store and leave a diagnostic breadcrumb in logs.
		common.SysLog("task idempotency redis unavailable, using local store: " + common.LocalLogPreview(err.Error()))
	}

	taskIdempotencyStore.Lock()
	defer taskIdempotencyStore.Unlock()
	now := time.Now()
	if entry, ok := taskIdempotencyStore.entries[key]; ok && now.Before(entry.expiresAt) {
		if strings.HasPrefix(entry.value, "done:") {
			taskID := strings.TrimPrefix(entry.value, "done:")
			task, exists, err := model.GetByTaskId(info.UserId, taskID)
			if err != nil {
				return nil, nil, service.TaskErrorWrapper(err, "idempotency_lookup_failed", 500)
			}
			if exists && task != nil {
				return task, nil, nil
			}
			delete(taskIdempotencyStore.entries, key)
		} else {
			return nil, nil, idempotencyInProgressError()
		}
	}
	taskIdempotencyStore.entries[key] = taskIdempotencyEntry{value: token, expiresAt: now.Add(taskIdempotencyTTL)}
	return nil, &taskIdempotencyLease{key: key, token: token}, nil
}

func idempotencyInProgressError() *dto.TaskError {
	return service.TaskErrorWrapperLocal(errors.New("an identical task request is already being processed"), "idempotency_in_progress", 409)
}

func resolveRedisIdempotency(key string, userID int) (*model.Task, bool) {
	value, err := common.RDB.Get(context.Background(), key).Result()
	if err != nil {
		return nil, false
	}
	if !strings.HasPrefix(value, "done:") {
		return nil, false
	}
	taskID := strings.TrimPrefix(value, "done:")
	task, exists, lookupErr := model.GetByTaskId(userID, taskID)
	if lookupErr != nil || !exists || task == nil {
		return nil, false
	}
	return task, true
}

func (l *taskIdempotencyLease) commit(taskID string) {
	if l == nil || taskID == "" {
		return
	}
	l.committed = true
	value := "done:" + taskID
	if l.redis && common.RDB != nil {
		_ = common.RDB.Set(context.Background(), l.key, value, taskIdempotencyTTL).Err()
		return
	}
	taskIdempotencyStore.Lock()
	taskIdempotencyStore.entries[l.key] = taskIdempotencyEntry{value: value, expiresAt: time.Now().Add(taskIdempotencyTTL)}
	taskIdempotencyStore.Unlock()
}

func (l *taskIdempotencyLease) release() {
	if l == nil || l.committed {
		return
	}
	if l.redis && common.RDB != nil {
		const compareAndDelete = `if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`
		_, _ = common.RDB.Eval(context.Background(), compareAndDelete, []string{l.key}, l.token).Result()
		return
	}
	taskIdempotencyStore.Lock()
	if entry, ok := taskIdempotencyStore.entries[l.key]; ok && entry.value == l.token {
		delete(taskIdempotencyStore.entries, l.key)
	}
	taskIdempotencyStore.Unlock()
}
