# 移动云素材库 curl 测试指南

本文用于在客户电脑、移动云 ECS 或网关服务器上，直接调用移动云素材库接口，确认网络、AK/SK 和签名是否正常。

网关用户接口说明见 [`/docs/api`](/docs/api)，管理员排错说明见
[`/docs/guide/admin`](/docs/guide/admin)。本文命令仅用于上游直连诊断，不替代网关鉴权。

> 这是移动云上游直连测试，不经过 new-api。视频生成渠道的 Bearer API Key 与素材库的 AccessKey/SecretKey 是两套凭证，不能混用。

## 1. 测试信息

| 项目 | 值 |
| --- | --- |
| Endpoint | `https://ecloud.10086.cn` |
| 资源池 | `CIDC-CORE-00` |
| 签名版本 | `V2.0` |
| 默认签名算法 | `HmacSHA1` |
| 查询素材组 | `POST /api/openapi-maas/exp/aicc/v2/asset-group/query` |
| 创建素材组 | `POST /api/openapi-maas/exp/aicc/v2/asset-group` |

素材组接口的资源池在账号和服务配置中确定，查询/创建请求体不需要额外添加 `poolId`。

## 2. 准备环境

需要准备：

- `curl`（建议使用支持 `--http1.1` 的版本）；
- Python 3.8 或更高版本，用于生成 V2.0 签名；
- 移动云素材库专用 `AccessKey` 和 `SecretKey`。

Linux、macOS、WSL：

```bash
export MOBILECLOUD_ACCESS_KEY='ACCESS_KEY'
export MOBILECLOUD_SECRET_KEY='SECRET_KEY'
```

Windows PowerShell：

```powershell
$env:MOBILECLOUD_ACCESS_KEY = "ACCESS_KEY"
$env:MOBILECLOUD_SECRET_KEY = "SECRET_KEY"
```

SecretKey 只放在本机环境变量中，不要提交到 Git、工单或公开聊天记录。

仓库同时提供了不回显密钥、默认只读查询的一键脚本。Windows 双击
`scripts\run_mobilecloud_asset_test.bat`，或在 PowerShell 执行：

```powershell
cd new-api
.\scripts\run_mobilecloud_asset_test.ps1
```

脚本会依次输出本机主机名、出口 IP、DNS、TLS 握手、上游 HTTP 状态和响应体；
请求采用直连 socket，不读取 `HTTP(S)_PROXY` 环境变量（代理软件的 TUN/VPN
路由仍会生效），因此输出的出口 IP 与实际探测请求一致；不会打印签名 URL、
SecretKey，也不会默认创建或删除素材组。需要验证创建/删除
时再显式执行 `--operation create --cleanup --yes`。

## 3. 第一步：无签名连通性探针

此请求只检查网络和接口入口，不会创建或修改素材组：

```bash
curl --http1.1 -i -X POST \
  'https://ecloud.10086.cn/api/openapi-maas/exp/aicc/v2/asset-group/query' \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  --data-raw '{"pageNo":1,"pageSize":1,"groupType":"AIGC"}' \
  -w '\nHTTP_STATUS:%{http_code}\nREMOTE_IP:%{remote_ip}\n'
```

无签名时预期返回 `400`，例如：

```json
{
  "errorMessage": "Input parameter AccessKey missing",
  "errorCode": "MISSING_PARAMETER"
}
```

这个结果表示请求已经到达移动云，接口路径和 HTTPS 入口正常。若直接出现 `Empty reply from server`、连接超时或 DNS 错误，应先排查出口线路和网络策略。

## 4. 第二步：生成签名 curl

将下面脚本保存为临时文件 `mobilecloud_curl.py`。脚本只生成命令，不会自行创建素材组；生成的签名应在几分钟内使用。

```python
import argparse
import datetime
import hashlib
import hmac
import json
import os
import shlex
import urllib.parse
import uuid


def percent_encode(value):
    return urllib.parse.quote(str(value), safe="-_.~")


def canonical_query(params):
    return "&".join(
        f"{percent_encode(key)}={percent_encode(params[key])}"
        for key in sorted(params)
    )


def signed_url(method, path, access_key, secret_key, signature_method):
    beijing = datetime.timezone(datetime.timedelta(hours=8))
    timestamp = datetime.datetime.now(beijing).strftime("%Y-%m-%dT%H:%M:%SZ")
    params = {
        "AccessKey": access_key,
        "Timestamp": timestamp,
        "SignatureNonce": uuid.uuid4().hex,
        "SignatureVersion": "V2.0",
        "SignatureMethod": signature_method,
    }

    canonical = canonical_query(params)
    query_hash = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    string_to_sign = (
        method.upper() + "\n" + percent_encode(path) + "\n" + query_hash
    )

    algorithm = hashlib.sha1
    if signature_method.upper() == "HMACSHA256":
        algorithm = hashlib.sha256

    signature = hmac.new(
        ("BC_SIGNATURE&" + secret_key).encode("utf-8"),
        string_to_sign.encode("utf-8"),
        algorithm,
    ).hexdigest()
    params["Signature"] = signature

    return (
        "https://ecloud.10086.cn"
        + path
        + "?"
        + canonical_query(params)
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--access-key", default=os.getenv("MOBILECLOUD_ACCESS_KEY"))
    parser.add_argument("--secret-key", default=os.getenv("MOBILECLOUD_SECRET_KEY"))
    parser.add_argument("--signature-method", default="HmacSHA1")
    parser.add_argument("--resolve-ip", default="")
    sub = parser.add_subparsers(dest="operation", required=True)

    query = sub.add_parser("query")
    query.add_argument("--group-id", default="")
    query.add_argument("--page-size", type=int, default=10)

    create = sub.add_parser("create")
    create.add_argument("--name", default="new-api-connectivity-test")
    create.add_argument("--description", default="temporary connectivity test")

    delete = sub.add_parser("delete")
    delete.add_argument("--group-id", required=True)

    args = parser.parse_args()
    if not args.access_key or not args.secret_key:
        raise SystemExit(
            "请先设置 MOBILECLOUD_ACCESS_KEY 和 MOBILECLOUD_SECRET_KEY"
        )

    if args.operation == "query":
        method = "POST"
        path = "/api/openapi-maas/exp/aicc/v2/asset-group/query"
        body = {"pageNo": 1, "pageSize": args.page_size, "groupType": "AIGC"}
        if args.group_id:
            body["groupIds"] = [args.group_id]
    elif args.operation == "create":
        method = "POST"
        path = "/api/openapi-maas/exp/aicc/v2/asset-group"
        body = {
            "groupType": "AIGC",
            "groupName": args.name,
            "description": args.description,
        }
    else:
        method = "DELETE"
        path = "/api/openapi-maas/exp/aicc/v2/asset-group/" + urllib.parse.quote(
            args.group_id, safe=""
        )
        body = None

    url = signed_url(
        method, path, args.access_key, args.secret_key, args.signature_method
    )
    command = ["curl", "--http1.1", "-i", "-X", method]
    if args.resolve_ip:
        command += ["--resolve", f"ecloud.10086.cn:443:{args.resolve_ip}"]
    command += [
        url,
        "-H", "Accept: application/json",
        "-w", "\\nHTTP_STATUS:%{http_code}\\nREMOTE_IP:%{remote_ip}\\n",
    ]
    if body is not None:
        command += [
            "-H", "Content-Type: application/json",
            "--data-raw",
            json.dumps(body, ensure_ascii=False, separators=(",", ":")),
        ]
    print(shlex.join(command))


if __name__ == "__main__":
    main()
```

## 5. 查询素材组

Linux/macOS/WSL：

```bash
python3 mobilecloud_curl.py query
```

Windows PowerShell：

```powershell
py .\mobilecloud_curl.py query
```

脚本会输出一条带完整签名参数的 `curl`。复制输出的命令并立即执行。

按指定节点测试时，可以使用：

```bash
python3 mobilecloud_curl.py query --resolve-ip 36.138.50.242
```

成功响应通常为 HTTP `200`，并包含：

```json
{
  "state": "OK",
  "requestId": "移动云请求ID",
  "body": {
    "data": [],
    "total": 0
  }
}
```

## 6. 创建测试素材组

创建只支持 `AIGC` 素材组：

```bash
python3 mobilecloud_curl.py create --name "new-api-test-001"
```

成功响应中记录移动云返回的 `body.groupId`：

```json
{
  "state": "OK",
  "body": {
    "groupId": "GROUP_ID",
    "groupType": "AIGC",
    "groupName": "new-api-test-001"
  }
}
```

使用返回的 ID 做精确查询：

```bash
python3 mobilecloud_curl.py query --group-id GROUP_ID
```

测试结束后删除临时素材组，避免留下无用资源：

```bash
python3 mobilecloud_curl.py delete --group-id GROUP_ID
```

## 7. 结果判断

| 现象 | 结论 |
| --- | --- |
| 无签名返回 `400 MISSING_PARAMETER` | 网络、域名、HTTPS 和接口入口正常 |
| 签名查询返回 `200`、`state=OK` | AK/SK、签名和请求路径正常 |
| 返回 `Invalid parameter Signature` | 检查 AK/SK、时间戳、签名算法和路径 |
| 返回 `Invalid parameter Timestamp` | 检查本机时间；重新生成签名后立即执行 |
| 返回 `400` 且提示请求体字段错误 | 网络和鉴权已通过，按响应修正 JSON 字段 |
| `Empty reply from server` | HTTP 响应前连接被关闭，重点检查出口线路、源 IP 或上游边缘策略 |
| 连接超时或 DNS 失败 | 检查服务器 DNS、防火墙和出站规则 |

## 8. 请客户回传的测试信息

请保留以下内容，提交给移动云技术支持或网关开发人员：

1. 执行环境：客户电脑、移动云 ECS 或网关服务器；
2. 执行时间（精确到秒）；
3. `HTTP_STATUS` 和 `REMOTE_IP`；
4. 移动云返回的完整 JSON（SecretKey 已隐藏）；
5. 若失败，附上 `curl -i` 的响应头和错误信息。

同一条签名请求最好分别从客户环境和网关服务器执行。客户环境成功、网关服务器出现 `Empty reply from server` 时，优先让移动云检查网关出口 IP `154.36.180.196` 的接入策略。
