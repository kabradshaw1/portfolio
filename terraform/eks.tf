# --- EKS Module ---
# Using the official AWS EKS module for best practices

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${var.project_name}-eks"
  cluster_version = var.eks_kubernetes_version

  vpc_id     = aws_vpc.main.id
  subnet_ids = aws_subnet.private[*].id

  # Public endpoint so kubectl works from Mac without VPN
  cluster_endpoint_public_access = true

  eks_managed_node_groups = {
    default = {
      instance_types = [var.eks_node_instance_type]
      min_size       = var.eks_node_count
      max_size       = var.eks_node_count + 1
      desired_size   = var.eks_node_count

      # Attach the node security group for managed service access
      vpc_security_group_ids = [aws_security_group.node.id]
    }
  }

  # Allow the GitHub Actions OIDC role to manage the cluster
  enable_cluster_creator_admin_permissions = true

  tags = {
    Project = var.project_name
  }
}
