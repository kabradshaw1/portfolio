# --- Default StorageClass (gp3 via EBS CSI) ---
# The application PVCs (ai-services ingestion-data/eval-data, monitoring
# prometheus/loki/pushgateway) do not set a storageClassName, so they bind to
# whichever StorageClass is marked cluster-default. EKS 1.31 ships a non-default
# in-tree "gp2" class, so we add a gp3 class and mark it default here.
#
# WaitForFirstConsumer delays volume creation until a pod is scheduled, so the
# EBS volume lands in the same AZ as the consuming pod (PVCs are zonal).

resource "kubernetes_storage_class" "gp3" {
  metadata {
    name = "gp3"
    annotations = {
      "storageclass.kubernetes.io/is-default-class" = "true"
    }
  }

  storage_provisioner    = "ebs.csi.aws.com"
  volume_binding_mode    = "WaitForFirstConsumer"
  allow_volume_expansion = true
  reclaim_policy         = "Delete"

  parameters = {
    type      = "gp3"
    encrypted = "true"
  }

  # The EBS CSI driver addon must be installed before its StorageClass is usable.
  depends_on = [module.eks]
}
