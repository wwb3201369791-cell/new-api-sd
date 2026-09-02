package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskIdempotencyLocalLeasePreventsConcurrentSubmit(t *testing.T) {
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request, _ = http.NewRequest(http.MethodPost, "/v1/videos", nil)
	ctx.Request.Header.Set(taskIdempotencyHeader, "demo-key")
	info := &relaycommon.RelayInfo{UserId: 42, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

	_, first, firstErr := beginTaskIdempotency(ctx, info)
	require.Nil(t, firstErr)
	require.NotNil(t, first)

	_, second, secondErr := beginTaskIdempotency(ctx, &relaycommon.RelayInfo{UserId: 42, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.Nil(t, second)
	require.Equal(t, "idempotency_in_progress", secondErr.Code)

	first.release()
	_, third, thirdErr := beginTaskIdempotency(ctx, &relaycommon.RelayInfo{UserId: 42, TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.Nil(t, thirdErr)
	require.NotNil(t, third)
	third.release()
}
