# --- AWS Secrets Manager ---
# Store sensitive values for K8s secrets injection at deploy time

resource "aws_secretsmanager_secret" "llm_api_key" {
  name                    = "${var.project_name}/llm-api-key"
  recovery_window_in_days = 0

  tags = {
    Name = "${var.project_name}-llm-api-key"
  }
}

resource "aws_secretsmanager_secret" "embedding_api_key" {
  name                    = "${var.project_name}/embedding-api-key"
  recovery_window_in_days = 0

  tags = {
    Name = "${var.project_name}-embedding-api-key"
  }
}
