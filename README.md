# 🚀 Bastion

> Event-Driven, Hash-Based Backup System for Kubernetes Custom Resources (CRs)

**Bastion** is a lightweight, high-performance backup controller that listens to Kubernetes Custom Resource (CR) events and triggers backups only when actual content changes. Built on hash comparison rather than full snapshots, bastion minimizes disk I/O and maximizes backup fidelity.

---

## ✨ Features

- ⚡ **Event-Driven**: Reacts to Add/Update/Delete events — no polling or scanning
- 🔒 **Content-Aware**: Backs up only when CR content changes, based on SHA-256 hashes
- 📁 **Granular & Structured**: Saves CRs in `/group/version/kind/namespace/name` layout
- 🧠 **Efficient**: Minimal memory and disk usage, perfect for edge and large clusters
- ♻️ **Crash-Tolerant**: Uses resync and reconciliation for recovery
- 🔌 **Scalable**: GVK-scoped informers and worker pools for horizontal scaling

---

## 📂 Backup Layout
```
/backups/apps/v1/Foo/ns1/foo-a/
  ├── manifest.yaml
  └── hash.txt
```

---

## 🛠️ Installation

```bash
kubectl apply -f https://raw.githubusercontent.com/your-org/bastion/main/deploy.yaml
```

---

## 🧪 How It Works

1. Watch for CR events (with annotation `backup/enabled: true`)
2. Sanitize and hash CR content
3. Compare with stored hash.txt
4. If changed → store `manifest.yaml` and update `hash.txt`

---

## 📈 Roadmap

- [ ] BackupPolicy CRD for scheduled/retained backups
- [ ] Support for Git/Azure Object upload
- [ ] CLI and REST interface for restore workflows
- [ ] Support for secret/ConfigMap encryption

---
