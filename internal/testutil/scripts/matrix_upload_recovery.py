"""Send twice through real Python Apprise to the local TLS recovery fixture."""

import json
import os
import sys
import tempfile

import apprise
from apprise import AppriseAsset, NotifyType

import capture_request


def main() -> None:
    # Trust only the fixture's certificate; retain normal HTTPS verification.
    os.environ["REQUESTS_CA_BUNDLE"] = sys.argv[2]
    capture_request.apply_store_fix()
    # Reuse the parity clock/transaction IDs without fixing cryptographic keys.
    capture_request.apply_fixed_time()
    with tempfile.TemporaryDirectory(prefix="matrix-recovery-") as storage:
        target = apprise.Apprise.instantiate(
            sys.argv[1], asset=AppriseAsset(storage_path=storage)
        )
        if target is None:
            raise RuntimeError("Matrix target was not constructed")
        outcomes = [
            bool(
                target.send(
                    body=f"body{number}",
                    title=f"title{number}",
                    notify_type=NotifyType.INFO,
                )
            )
            for number in range(2)
        ]
    print(json.dumps(outcomes))


if __name__ == "__main__":
    main()
