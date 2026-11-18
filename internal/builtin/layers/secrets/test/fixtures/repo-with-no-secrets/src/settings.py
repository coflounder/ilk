"""Configuration.

Nothing here is a secret. Every value that is one comes from the environment,
which is the arrangement that makes the scanner boring to live with.
"""

import os

ACME_API_TOKEN = os.environ["ACME_API_TOKEN"]
AWS_REGION = os.environ.get("AWS_REGION", "eu-west-1")

# Forty hexadecimal characters, and not a credential. A scanner that flags this
# is a scanner people will start bypassing.
BUILD_DIGEST = "9f2c1a4e8b7d6f3a0c5e2b8d4a7f1c9e3b6d0a5f"
