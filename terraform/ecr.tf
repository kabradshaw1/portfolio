# --- ECR Repositories ---
# One repository per application service image that the aws/ kustomize overlays
# deploy. Third-party images (postgres, redis, mongo, qdrant, kafka, rabbitmq,
# exporters) are pulled from public registries and do not need a repo here.

locals {
  services = [
    # AI services (services/, Python). eval also backs the eval-worker deployment.
    "ingestion",
    "chat",
    "debug",
    "eval",
    # Java services (java/)
    "java-task-service",
    "java-activity-service",
    "java-notification-service",
    "java-gateway-service",
    # Go services (go/). The former go-ecommerce monolith was split into these.
    "go-auth-service",
    "go-order-service",
    "go-analytics-service",
    "go-product-service",
    "go-cart-service",
    "go-payment-service",
    "go-order-projector",
    "go-ai-service",
  ]
}

resource "aws_ecr_repository" "services" {
  for_each = toset(local.services)

  name                 = "${var.project_name}/${each.key}"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Name = each.key
  }
}

resource "aws_ecr_lifecycle_policy" "services" {
  for_each = aws_ecr_repository.services

  repository = each.value.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 5 images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 5
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}
