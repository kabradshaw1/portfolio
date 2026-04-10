# --- Amazon MQ (RabbitMQ) ---

resource "aws_mq_broker" "rabbitmq" {
  broker_name = "${var.project_name}-rabbitmq"

  engine_type        = "RabbitMQ"
  engine_version     = "3.13"
  host_instance_type = var.mq_instance_type
  deployment_mode    = "SINGLE_INSTANCE"

  user {
    username = var.mq_username
    password = var.mq_password
  }

  subnet_ids          = [aws_subnet.private[0].id]
  security_groups     = [aws_security_group.mq.id]
  publicly_accessible = false

  tags = {
    Name = "${var.project_name}-rabbitmq"
  }
}
