#!/usr/bin/env python3
"""Live integration smoke test for the Backend Golang Coding Test API.

Requires Python 3 standard library only and a RUNNING API connected to MongoDB.
Creates uniquely named test users and removes them when possible.
Run: python3 scripts/smoke_test.py --base-url http://localhost:8080
"""

import argparse
import concurrent.futures
import json
import sys
import urllib.error
import urllib.request
import uuid
from dataclasses import dataclass

PASSWORD = "smoke-test-password-123"


@dataclass
class Result:
    status: int
    data: object
    text: str


class API:
    def __init__(self, base_url, timeout):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def call(self, method, path, body=None, token=None, raw=False):
        headers = {"Accept": "application/json"}
        if token is not None:
            headers["Authorization"] = "Bearer " + token
        data = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            data = body.encode("utf-8") if raw else json.dumps(body).encode("utf-8")
        request = urllib.request.Request(
            self.base_url + path, data=data, headers=headers, method=method
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                status = response.status
                text = response.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            status = exc.code
            text = exc.read().decode("utf-8")
        parsed = None
        if text:
            try:
                parsed = json.loads(text)
            except json.JSONDecodeError as exc:
                raise AssertionError(
                    f"Non-JSON response for {method} {path}: HTTP {status}: {text[:300]}"
                ) from exc
        return Result(status=status, data=parsed, text=text)


def expect(result, status, desc):
    if result.status != status:
        raise AssertionError(
            f"{desc}: expected HTTP {status}, got {result.status}: {result.text[:500]}"
        )
    print(f"PASS  {desc} (HTTP {status})")
    return result.data


def required(mapping, key, desc):
    if not isinstance(mapping, dict) or not mapping.get(key):
        raise AssertionError(f"{desc}: expected non-empty key {key!r}: {mapping!r}")
    return mapping[key]


def assert_no_secret(response, desc):
    if PASSWORD in response.text or "password_hash" in response.text:
        raise AssertionError(f"{desc}: response leaked password/hash")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--concurrency", type=int, default=30)
    args = parser.parse_args()
    if args.timeout <= 0 or args.concurrency < 2 or args.concurrency > 100:
        parser.error("--timeout must be > 0; --concurrency must be 2..100")

    api = API(args.base_url, args.timeout)
    ident = uuid.uuid4().hex[:12]
    prefix = f"assessment-smoke-{ident}"
    email_a = f"{prefix}-a@example.com"
    email_a_new = f"{prefix}-a-updated@example.com"
    email_b = f"{prefix}-b@example.com"
    email_c = f"{prefix}-race@example.com"
    email_d = f"{prefix}-auth-created@example.com"
    accounts = []  # (user id, token), for best-effort cleanup
    try:
        health = expect(api.call("GET", "/healthz"), 200, "health endpoint")
        if health.get("status") != "ok":
            raise AssertionError("health status is not ok")
        expect(api.call("GET", "/api/v1/users"), 401, "missing JWT rejected")
        expect(api.call("GET", "/api/v1/users", token="invalid.token.here"), 401,
               "malformed JWT rejected")
        expect(api.call("POST", "/api/v1/auth/register",
                        {"name": "Invalid", "email": "invalid", "password": PASSWORD}),
               400, "email validation")
        expect(api.call("POST", "/api/v1/auth/register",
                        '{"name":"Invalid","email":"bad@example.com",', raw=True),
               400, "malformed JSON rejected")
        expect(api.call("POST", "/api/v1/auth/register",
                        {"name": "Invalid", "email": f"{prefix}-unknown@example.com",
                         "password": PASSWORD, "unexpected": 1}),
               400, "unknown JSON field rejected")

        a = api.call("POST", "/api/v1/auth/register",
                     {"name": "Alice Smoke", "email": email_a.upper(), "password": PASSWORD})
        user_a = required(expect(a, 201, "register A"), "user", "register A")
        id_a = required(user_a, "id", "register A")
        if user_a["email"] != email_a:
            raise AssertionError("email normalization failed")
        assert_no_secret(a, "register A")
        expect(api.call("POST", "/api/v1/auth/register",
                        {"name": "Alice Duplicate", "email": email_a, "password": PASSWORD}),
               409, "duplicate email rejected")
        expect(api.call("POST", "/api/v1/auth/login",
                        {"email": email_a, "password": "incorrect-password"}),
               401, "incorrect password rejected")

        login_a = expect(api.call("POST", "/api/v1/auth/login",
                                  {"email": email_a, "password": PASSWORD}),
                         200, "login A")
        token_a = required(login_a, "access_token", "login A")
        accounts.append((id_a, token_a))

        b = api.call("POST", "/api/v1/auth/register",
                     {"name": "Bob Smoke", "email": email_b, "password": PASSWORD})
        user_b = required(expect(b, 201, "register B"), "user", "register B")
        id_b = required(user_b, "id", "register B")
        login_b = expect(api.call("POST", "/api/v1/auth/login",
                                  {"email": email_b, "password": PASSWORD}),
                         200, "login B")
        token_b = required(login_b, "access_token", "login B")
        accounts.append((id_b, token_b))

        created = api.call("POST", "/api/v1/users",
                           {"name": "Created via Protected API", "email": email_d,
                            "password": PASSWORD}, token=token_a)
        user_d = required(expect(created, 201, "authenticated POST /users"),
                          "user", "create D")
        id_d = required(user_d, "id", "create D")
        assert_no_secret(created, "authenticated create")
        login_d = expect(api.call("POST", "/api/v1/auth/login",
                                  {"email": email_d, "password": PASSWORD}),
                         200, "login user D")
        token_d = required(login_d, "access_token", "login D")
        accounts.append((id_d, token_d))

        fetched = api.call("GET", f"/api/v1/users/{id_a}", token=token_a)
        expect(fetched, 200, "GET user A")
        assert_no_secret(fetched, "GET user A")
        listing = api.call("GET", "/api/v1/users?page=1&limit=100", token=token_a)
        expect(listing, 200, "list users")
        if not isinstance(listing.data.get("users"), list):
            raise AssertionError("list response must contain users array")
        assert_no_secret(listing, "list users")
        expect(api.call("GET", "/api/v1/users/not-an-id", token=token_a),
               400, "invalid ObjectID rejected")
        expect(api.call("GET", "/api/v1/users?page=-1", token=token_a),
               400, "invalid pagination rejected")
        expect(api.call("PATCH", f"/api/v1/users/{id_a}",
                        {"name": "Evil Update"}, token=token_b),
               403, "cross-user update denied")
        expect(api.call("DELETE", f"/api/v1/users/{id_a}", token=token_b),
               403, "cross-user delete denied")
        expect(api.call("PATCH", f"/api/v1/users/{id_a}",
                        {"email": email_b}, token=token_a),
               409, "update to duplicate email denied")
        update = api.call("PATCH", f"/api/v1/users/{id_a}",
                          {"name": "Alice Updated", "email": email_a_new}, token=token_a)
        updated_user = required(expect(update, 200, "update own account"),
                                "user", "update A")
        if updated_user.get("name") != "Alice Updated" or updated_user.get("email") != email_a_new:
            raise AssertionError("update returned unexpected data")
        assert_no_secret(update, "update A")
        expect(api.call("POST", "/api/v1/auth/login",
                        {"email": email_a_new, "password": PASSWORD}),
               200, "login using new email")

        # Exercise the MongoDB unique index in the presence of concurrent requests.
        # Each request uses the SAME canonical email, not merely the same number.
        def concurrent_register(_):
            return api.call("POST", "/api/v1/auth/register",
                            {"name": "Race Smoke", "email": email_c, "password": PASSWORD}).status

        with concurrent.futures.ThreadPoolExecutor(max_workers=min(30, args.concurrency)) as pool:
            statuses = list(pool.map(concurrent_register, range(args.concurrency)))
        if statuses.count(201) != 1 or statuses.count(409) != args.concurrency - 1:
            raise AssertionError(
                f"duplicate-email race: expected exactly one HTTP 201 and "
                f"{args.concurrency - 1} HTTP 409; got {statuses}"
            )
        print(f"PASS  concurrent duplicate-email test ({args.concurrency} requests: 1 created)")
        login_c = expect(api.call("POST", "/api/v1/auth/login",
                                  {"email": email_c, "password": PASSWORD}),
                         200, "login race winner")
        token_c = required(login_c, "access_token", "login C")
        # The C id can be determined from the 'sub' of the JWT only with manual decode,
        # or from a /users listing. To avoid inspecting unauthenticated token payloads,
        # scan bounded pagination by unique email.
        page = 1
        id_c = None
        while True:
            result = expect(api.call("GET", f"/api/v1/users?page={page}&limit=100",
                                     token=token_c), 200, "list for race cleanup")
            users = result.get("users", [])
            id_c = next((u["id"] for u in users if u.get("email") == email_c), None)
            if id_c or len(users) < 100:
                break
            page += 1
            if page > 100:
                break
        if id_c:
            accounts.append((id_c, token_c))
        else:
            print("WARN  race winner ID not found within first 10k users; manual cleanup may be needed")

        expect(api.call("DELETE", f"/api/v1/users/{id_a}", token=token_a),
               204, "delete own account")
        accounts = [(id_, token) for id_, token in accounts if id_ != id_a]
        expect(api.call("GET", "/api/v1/users", token=token_a),
               401, "deleted user's JWT rejected")
        expect(api.call("GET", f"/api/v1/users/{id_a}", token=token_b),
               404, "deleted account is absent")
        print("\nSUCCESS: Live HTTP + MongoDB smoke checks passed.")
        print("NOTE: Background 10-second logs and SIGTERM graceful shutdown must be verified separately.")
    finally:
        for user_id, token in accounts:
            try:
                result = api.call("DELETE", f"/api/v1/users/{user_id}", token=token)
                if result.status != 204:
                    print(f"WARN  cleanup of {user_id} returned HTTP {result.status}", file=sys.stderr)
            except Exception as exc:
                print(f"WARN  cleanup of {user_id} failed: {exc}", file=sys.stderr)


if __name__ == "__main__":
    try:
        main()
    except (AssertionError, urllib.error.URLError, TimeoutError) as error:
        print(f"\nFAILED: {error}", file=sys.stderr)
        sys.exit(1)
