#!/usr/bin/env python3
"""Read-only Mobile Cloud asset API connectivity test.

The script intentionally does not print the signed URL or credentials.  It
performs the same V2.0 query-signature request as the gateway and reports
whether the request reached the upstream service.
"""

from __future__ import annotations

import argparse
import datetime as dt
import getpass
import hashlib
import hmac
import http.client
import json
import os
import socket
import ssl
import sys
import urllib.parse
import urllib.request
import uuid
from typing import Any


DEFAULT_BASE_URL = "https://ecloud.10086.cn"
QUERY_PATH = "/api/openapi-maas/exp/aicc/v2/asset-group/query"
CREATE_PATH = "/api/openapi-maas/exp/aicc/v2/asset-group"
DELETE_PATH_PREFIX = "/api/openapi-maas/exp/aicc/v2/asset-group/"
MAX_RESPONSE_BYTES = 8 * 1024 * 1024


def percent_encode(value: Any) -> str:
    return urllib.parse.quote(str(value), safe="-_.~")


def canonical_query(params: dict[str, Any], include_signature: bool = False) -> str:
    keys = sorted(
        key
        for key in params
        if include_signature or key.lower() != "signature"
    )
    return "&".join(
        f"{percent_encode(key)}={percent_encode(params[key])}" for key in keys
    )


def signed_query(
    method: str,
    path: str,
    access_key: str,
    secret_key: str,
    signature_method: str,
) -> str:
    beijing = dt.timezone(dt.timedelta(hours=8))
    timestamp = dt.datetime.now(dt.timezone.utc).astimezone(beijing).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )
    params: dict[str, Any] = {
        "AccessKey": access_key,
        "Timestamp": timestamp,
        "SignatureNonce": uuid.uuid4().hex,
        "SignatureVersion": "V2.0",
        "SignatureMethod": signature_method,
    }
    canonical = canonical_query(params)
    query_hash = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    string_to_sign = (
        method.upper()
        + "\n"
        + percent_encode(path)
        + "\n"
        + query_hash
    )
    algorithm = hashlib.sha256 if signature_method.upper() == "HMACSHA256" else hashlib.sha1
    signature = hmac.new(
        ("BC_SIGNATURE&" + secret_key).encode("utf-8"),
        string_to_sign.encode("utf-8"),
        algorithm,
    ).hexdigest()
    params["Signature"] = signature
    return canonical_query(params, include_signature=True)


def parse_base_url(raw: str) -> tuple[str, int, str]:
    parsed = urllib.parse.urlparse(raw.rstrip("/"))
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise ValueError("基础 URL 必须是完整的 http(s) 地址")
    if parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
        raise ValueError("基础 URL 不应包含路径、查询参数或片段")
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    return parsed.scheme, port, parsed.hostname


def resolve_addresses(host: str, port: int) -> list[str]:
    addresses: list[str] = []
    for family, _, _, _, sockaddr in socket.getaddrinfo(
        host, port, type=socket.SOCK_STREAM
    ):
        address = sockaddr[0]
        if address not in addresses:
            addresses.append(address)
    return addresses


def show_egress_ip() -> None:
    try:
        # The request below uses a raw socket so that the probe describes the
        # same direct route as http_request. Do not let HTTP(S)_PROXY alter
        # this diagnostic value; TUN/VPN routing is still reflected naturally.
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open("https://api.ipify.org", timeout=5) as response:
            value = response.read(128).decode("ascii", "replace").strip()
        print(f"EGRESS_IP: {value or 'unknown'}")
    except Exception as exc:  # noqa: BLE001 - diagnostic output must continue
        print(f"EGRESS_IP: unknown ({type(exc).__name__})")


def show_proxy_environment() -> None:
    names = [
        name
        for name in (
            "HTTP_PROXY",
            "HTTPS_PROXY",
            "ALL_PROXY",
            "http_proxy",
            "https_proxy",
            "all_proxy",
        )
        if os.getenv(name)
    ]
    if names:
        print("PROXY_ENV: configured (direct socket probe ignores these variables)")
    else:
        print("PROXY_ENV: none")


def tls_probe(
    scheme: str,
    host: str,
    port: int,
    target_ip: str,
    timeout: float,
) -> None:
    if scheme != "https":
        print("TLS: skipped (base URL uses HTTP)")
        return
    raw = None
    wrapped = None
    try:
        raw = socket.create_connection((target_ip, port), timeout=timeout)
        wrapped = ssl.create_default_context().wrap_socket(raw, server_hostname=host)
        print(
            "TLS: ok"
            f" version={wrapped.version()}"
            f" cipher={wrapped.cipher()[0] if wrapped.cipher() else 'unknown'}"
        )
    except Exception as exc:  # noqa: BLE001 - this is a connectivity probe
        print(f"TLS: failed ({type(exc).__name__}: {exc})")
    finally:
        if wrapped is not None:
            wrapped.close()
        elif raw is not None:
            raw.close()


def http_request(
    scheme: str,
    host: str,
    port: int,
    target_ip: str,
    method: str,
    path: str,
    query: str,
    body: bytes | None,
    timeout: float,
) -> tuple[int | None, str, bytes | None, str | None]:
    request_target = path + ("?" + query if query else "")
    content = body or b""
    headers = [
        f"{method.upper()} {request_target} HTTP/1.1",
        f"Host: {host}" + (f":{port}" if port not in {80, 443} else ""),
        "Accept: application/json",
        "Connection: close",
    ]
    if body is not None:
        headers.extend(
            [
                "Content-Type: application/json",
                f"Content-Length: {len(content)}",
            ]
        )
    headers.extend(["", ""])
    wire_request = ("\r\n".join(headers)).encode("ascii") + content

    raw = None
    wrapped = None
    peer_ip = target_ip
    try:
        raw = socket.create_connection((target_ip, port), timeout=timeout)
        if scheme == "https":
            wrapped = ssl.create_default_context().wrap_socket(
                raw, server_hostname=host
            )
            connection_socket = wrapped
        else:
            connection_socket = raw
        peer_ip = connection_socket.getpeername()[0]
        connection_socket.settimeout(timeout)
        connection_socket.sendall(wire_request)
        response = http.client.HTTPResponse(connection_socket, method=method.upper())
        response.begin()
        data = response.read(MAX_RESPONSE_BYTES)
        return response.status, peer_ip, data, None
    except Exception as exc:  # noqa: BLE001 - preserve exact network diagnosis
        return None, peer_ip, None, f"{type(exc).__name__}: {exc}"
    finally:
        if wrapped is not None:
            wrapped.close()
        elif raw is not None:
            raw.close()


def read_credentials(args: argparse.Namespace) -> tuple[str, str]:
    access_key = (args.access_key or os.getenv("MOBILECLOUD_ACCESS_KEY", "")).strip()
    secret_key = (os.getenv("MOBILECLOUD_SECRET_KEY", "")).strip()
    if not access_key:
        access_key = input("移动云素材 AccessKey: ").strip()
    if not secret_key:
        secret_key = getpass.getpass("移动云素材 SecretKey（输入不回显）: ").strip()
    if not access_key or not secret_key:
        raise ValueError(
            "AccessKey/SecretKey 不能为空；也可设置 MOBILECLOUD_ACCESS_KEY 和 "
            "MOBILECLOUD_SECRET_KEY 环境变量"
        )
    return access_key, secret_key


def operation_request(args: argparse.Namespace) -> tuple[str, str, dict[str, Any] | None]:
    operation = args.operation
    if operation == "query":
        body: dict[str, Any] = {
            "pageNo": 1,
            "pageSize": args.page_size,
            "groupType": "AIGC",
        }
        if args.group_id:
            body["groupIds"] = [args.group_id]
        return "POST", QUERY_PATH, body
    if operation == "create":
        name = args.name or (
            "new-api-local-test-"
            + dt.datetime.now().strftime("%Y%m%d%H%M%S")
        )
        return "POST", CREATE_PATH, {
            "groupType": "AIGC",
            "groupName": name,
            "description": args.description,
        }
    if not args.group_id:
        raise ValueError("delete 操作必须提供 --group-id")
    return "DELETE", DELETE_PATH_PREFIX + urllib.parse.quote(args.group_id, safe=""), None


def extract_group_id(value: Any) -> str:
    if isinstance(value, dict):
        for key in ("groupId", "groupID", "id"):
            candidate = value.get(key)
            if isinstance(candidate, str) and candidate.strip():
                return candidate.strip()
        for key in ("body", "data", "result", "response"):
            nested = value.get(key)
            found = extract_group_id(nested)
            if found:
                return found
    return ""


def print_response(status: int | None, peer_ip: str, data: bytes | None, error: str | None) -> None:
    print(f"UPSTREAM_IP: {peer_ip}")
    if error:
        print(f"HTTP_STATUS: 000")
        print(f"NETWORK_ERROR: {error}")
        print(
            "判断: 请求在收到 HTTP 响应前失败；请检查出口 IP、DNS/TLS、WAF 或移动云接入策略。"
        )
        return
    print(f"HTTP_STATUS: {status}")
    text = (data or b"").decode("utf-8", "replace")
    try:
        parsed = json.loads(text)
        print("RESPONSE_JSON:")
        print(json.dumps(parsed, ensure_ascii=False, indent=2))
    except json.JSONDecodeError:
        print("RESPONSE_BODY:")
        print(text[:4000])
    if status is not None and 200 <= status < 300:
        print("判断: 移动云网络、签名、路径和请求体均已通过。")
    elif status is not None and 400 <= status < 500:
        print("判断: 已到达移动云；请根据返回的 errorCode/errorMessage 修正凭证或参数。")
    else:
        print("判断: 已收到移动云响应，属于上游服务端错误。")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="移动云素材库 V2.0 签名连通性测试（默认只查询，不创建资源）"
    )
    parser.add_argument("--base-url", default=os.getenv("MOBILECLOUD_BASE_URL", DEFAULT_BASE_URL))
    parser.add_argument("--access-key", default="", help="不建议写入命令行，可使用环境变量或交互输入")
    parser.add_argument("--signature-method", choices=["HmacSHA1", "HmacSHA256"], default="HmacSHA1")
    parser.add_argument("--resolve-ip", default="", help="指定移动云节点 IP，保留 Host/SNI")
    parser.add_argument("--timeout", type=float, default=15.0)
    parser.add_argument("--page-size", type=int, default=1)
    parser.add_argument("--group-id", default="")
    parser.add_argument("--name", default="")
    parser.add_argument("--description", default="temporary local connectivity test")
    parser.add_argument("--yes", action="store_true", help="创建素材组时跳过确认")
    parser.add_argument("--cleanup", action="store_true", help="创建成功后立即删除测试素材组")
    parser.add_argument("--operation", choices=["query", "create", "delete"], default="query")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        scheme, port, host = parse_base_url(args.base_url)
        if args.timeout <= 0:
            raise ValueError("--timeout 必须大于 0")
        if args.page_size < 1 or args.page_size > 100:
            raise ValueError("--page-size 必须在 1 到 100 之间")
        if args.operation == "create" and not args.yes:
            answer = input("将创建一个 AIGC 测试素材组，继续吗？[y/N] ").strip().lower()
            if answer not in {"y", "yes"}:
                print("已取消创建操作。")
                return 0
        access_key, secret_key = read_credentials(args)
        method, path, payload = operation_request(args)
    except (ValueError, EOFError, KeyboardInterrupt) as exc:
        print(f"参数错误: {exc}", file=sys.stderr)
        return 2

    print(f"HOST: {host}")
    print(f"OPERATION: {args.operation}")
    print(f"REQUEST_PATH: {path}")
    print(f"SIGNATURE_METHOD: {args.signature_method}")
    print(f"LOCAL_HOSTNAME: {socket.gethostname()}")
    show_proxy_environment()
    show_egress_ip()
    try:
        addresses = resolve_addresses(host, port)
    except socket.gaierror as exc:
        print(f"DNS: failed ({exc})")
        return 3
    if not addresses:
        print("DNS: no address returned")
        return 3
    print("DNS: " + ", ".join(addresses))
    target_ip = args.resolve_ip.strip() or next(
        (address for address in addresses if ":" not in address), addresses[0]
    )
    print(f"TARGET_IP: {target_ip}")
    tls_probe(scheme, host, port, target_ip, args.timeout)

    query = signed_query(method, path, access_key, secret_key, args.signature_method)
    body = (
        json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        if payload is not None
        else None
    )
    status, peer_ip, data, error = http_request(
        scheme,
        host,
        port,
        target_ip,
        method,
        path,
        query,
        body,
        args.timeout,
    )
    print_response(status, peer_ip, data, error)
    if error:
        return 10
    if status is None:
        return 10
    if status < 200 or status >= 300:
        return 11

    if args.operation == "create" and args.cleanup:
        try:
            response_value = json.loads((data or b"{}").decode("utf-8", "replace"))
        except json.JSONDecodeError:
            response_value = {}
        group_id = extract_group_id(response_value)
        if not group_id:
            print("CLEANUP: 未从创建响应中找到 groupId，请根据响应手动删除。")
            return 12
        print(f"CREATED_GROUP_ID: {group_id}")
        delete_query = signed_query(
            "DELETE", DELETE_PATH_PREFIX + urllib.parse.quote(group_id, safe=""),
            access_key, secret_key, args.signature_method
        )
        delete_status, delete_peer, delete_data, delete_error = http_request(
            scheme,
            host,
            port,
            target_ip,
            "DELETE",
            DELETE_PATH_PREFIX + urllib.parse.quote(group_id, safe=""),
            delete_query,
            None,
            args.timeout,
        )
        print("CLEANUP_RESULT:")
        print_response(delete_status, delete_peer, delete_data, delete_error)
        if delete_error or delete_status is None or not 200 <= delete_status < 300:
            return 12
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
