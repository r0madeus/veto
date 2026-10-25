# 👑 Veto

[![Test](https://github.com/r0madeus/veto/actions/workflows/test.yml/badge.svg)](https://github.com/r0madeus/veto/actions/workflows/test.yml)
[![Lint](https://github.com/r0madeus/veto/actions/workflows/lint.yml/badge.svg)](https://github.com/r0madeus/veto/actions/workflows/lint.yml)
[![Security Scan](https://github.com/r0madeus/veto/actions/workflows/security.yml/badge.svg)](https://github.com/r0madeus/veto/actions/workflows/security.yml)
[![E2E Tests](https://github.com/r0madeus/veto/actions/workflows/e2e.yml/badge.svg)](https://github.com/r0madeus/veto/actions/workflows/e2e.yml)
![Go Version](https://img.shields.io/github/go-mod/go-version/r0madeus/veto)
[![License](https://img.shields.io/github/license/r0madeus/veto)](LICENSE)

> **The Sovereign System Orchestrator**
> _"Rule your system through declarative architecture."_

**Veto** is an experimental system orchestrator designed to transform chaotic Linux management into a **declarative, state-driven experience**. It treats your OS as a collection of modular **Profiles**, allowing you to define, apply, and monitor your system state using simple YAML.

Veto aims to provide the declarative power of NixOS with the flexibility of a traditional rolling-release distribution like Arch or Fedora.

> [!WARNING]
> **Experimental (Alpha Stage):** Veto is currently in active development. While core features like file management and system discovery are stable, advanced features like automatic package rollback are experimental. Always back up your data.

---

## ⚡ The Philosophy

Modern Linux setups suffer from **"System Drift."** Manual changes via `pacman`, `dotfile` symlinks, and `systemctl` make a system "dirty" over time. 

Veto's goal is to become the **Single Source of Truth** for:
1. **Packages** (Cross-distro management)
2. **Configurations** (Templates & Symlinks)
3. **Services** (Lifecycle management)
4. **Secrets** (Encrypted sensitive data)

---

## 🆚 Veto vs. The Ecosystem

| **Feature** | **👑 Veto** | **❄️ NixOS** | **🐍 Ansible** | **📂 Chezmoi** |
| :--- | :--- | :--- | :--- | :--- |
| **Approach** | Declarative + Distro Agnostic | Entire OS Reconstruction | Remote Configuration | Dotfile Only |
| **Stability** | Distro Base + Atomic Snaps | Immutable Hash-based | Procedural | None |
| **State Tracking** | ✅ JSON-based Tracking | ✅ Nix Store | ❌ Manual | ❌ None |
| **BTRFS Rollback** | ✅ Integrated (Optional) | ✅ Native | ❌ No | ❌ No |
| **Learning Curve** | 🟢 Low (YAML) | 🔴 Very High | 🟡 Medium | 🟢 Low |

---

## 🚀 Current Capabilities

### 1. **Context-Aware System Awareness**
Instead of static scripts, Veto detects your hardware (CPU, GPU) and distribution details. These details are injected into templates, allowing you to create one config that works on both your AMD laptop and NVIDIA workstation.

### 2. **Baseline Discovery & Import**
Moving to Veto is not a manual task. Running `veto import` scans your current system state—explicitly installed packages and active services—and generates a baseline configuration to help you migrate.

### 3. **Live Watch & Iteration**
Designed for power users, `veto watch` monitors your configuration files. When you save a change, Veto automatically synchronizes the system—perfect for iterative styling of your desktop environment or testing new service configs.

### 4. **Built-in Secret Management**
Handle sensitive data without leaking it. Veto provides a native encryption layer using a master key, allowing you to store encrypted strings directly in your Git-tracked YAML files.

### 5. **Atomic Hook (BTRFS)**
When running on BTRFS, Veto can automatically trigger `snapper` or `timeshift` snapshots before applying changes. This provides an external safety net beyond Veto's internal state tracking.

---

## 🏗️ Supported Adapters (Overview)

Veto uses a modular adapter system to talk to your OS components:

- **Package Managers:** `pacman`, `yay`, `paru`, `apt`, `dnf`, `yum`, `zypper`, `brew`, `apk`, `flatpak`, `snap`.
- **Init Systems:** `systemd`, `openrc`, `sysvinit`.
- **Filesystem:** Templates (Go-style), Symlinks, Line-in-file, Archives.
- **Automation:** Git integration, User/Group management, Shell execution.

---

## 📦 Quick Start

### 1. Initialize Context
```bash
veto init
```

### 2. Discover Your System
```bash
veto import my-system.yaml
```

### 3. Apply & Watch
```bash
# Preview changes
veto apply my-system.yaml --dry-run

# Apply and track state
veto apply my-system.yaml

# Hot-reload on file change
veto watch my-system.yaml
```

---

## �️ Roadmap & Stability

| Feature | Status | Description |
| :--- | :--- | :--- |
| **State Tracking** | ✅ Stable | JSON-based tracking of applied resources. |
| **File/Template** | ✅ Stable | Declarative file management and symlinking. |
| **Secrets** | ✅ Stable | AES encryption for sensitive YAML values. |
| **Discovery** | 🟡 Beta | Accurate for Arch/Debian; generic for others. |
| **Rollback** | 🟡 Experimental | File restoration is stable; Pkg revert is WIP. |
| **Hub** | ⏳ Planned | Community registry for sharing profiles/rulesets. |
| **Veto Studio** | 🔮 Vision | GUI dashboard for visual orchestration. |

---

## 🤝 Contributing

We are building a community-driven tool. Check the `issues` page for tasks related to resource adapters or core engine optimization.

---

## 📜 License

Distributed under the Apache 2.0 License. See `LICENSE` for more information.

**Veto** © 2025 Developed by **Melih Uçgun**
_"Infrastructure is sovereignty."_
