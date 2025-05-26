# bastion
Bastion is a lightweight, zero-loss, and event-driven backup solution purpose-built for Kubernetes Custom Resources (CRs). Instead of relying on periodic snapshots like traditional tools (e.g., Velero), bastion intelligently watches for actual changes and backs up only when content changes-using cryptographic hash comparison to detect differences.
