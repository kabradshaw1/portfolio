#!/usr/bin/env bash
set -euo pipefail

# Tear down the AWS environment after a demo.
# Keeps the S3 state bucket (~$0.01/month) and ECR images (~$0.10/month).
# Usage: ./scripts/aws-down.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TF_DIR="$REPO_DIR/terraform"

echo "==> Destroying AWS infrastructure..."
echo "    This will remove: EKS, RDS, ElastiCache, Amazon MQ, ALB, VPC"
echo "    This will keep:   S3 state bucket, ECR images, MongoDB Atlas"
echo ""

cd "$TF_DIR"
terraform destroy -auto-approve

echo ""
echo "============================================"
echo "  AWS environment is down!"
echo "============================================"
echo ""
echo "  Next steps:"
echo "    1. In Cloudflare, restore api.kylebradshaw.dev -> Tunnel"
echo "    2. Unpause the Cloudflare Tunnel to the Windows PC"
echo ""
echo "  Residual costs: ~\$0.11/month (S3 bucket + ECR images)"
echo "  To spin up again: ./scripts/aws-up.sh"
echo ""
