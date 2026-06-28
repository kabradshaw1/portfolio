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

  # Managed addons. The EBS CSI driver is required for any PersistentVolumeClaim
  # to provision an EBS volume — without it every PVC (qdrant, ingestion-data,
  # eval-data, mongodb, and the monitoring stack) stays Pending forever. The
  # driver's controller service account assumes the IRSA role defined below.
  cluster_addons = {
    aws-ebs-csi-driver = {
      most_recent              = true
      service_account_role_arn = module.ebs_csi_irsa.iam_role_arn
    }
  }

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

# --- EBS CSI Driver IRSA Role ---
# Grants the aws-ebs-csi-driver controller (kube-system:ebs-csi-controller-sa)
# permission to create/attach/delete EBS volumes for PersistentVolumeClaims.

module "ebs_csi_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.0"

  role_name = "${var.project_name}-ebs-csi"

  attach_ebs_csi_policy = true

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:ebs-csi-controller-sa"]
    }
  }
}
