# Fake GPU Operator

<div align="center">

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/run-ai/fake-gpu-operator)](https://github.com/run-ai/fake-gpu-operator/releases)
[![CI](https://github.com/run-ai/fake-gpu-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/run-ai/fake-gpu-operator/actions/workflows/ci.yml)

🎮 Simulate NVIDIA GPUs in Kubernetes without actual hardware
</div>

> **Note:** We have moved our container images to GitHub Container Registry (GHCR). All images are now available at `ghcr.io/run-ai/fake-gpu-operator/*`. See the [Installation](#installation) section for details.

## 🚀 Overview

The Fake GPU Operator is a lightweight tool that simulates NVIDIA GPUs in Kubernetes clusters without requiring physical hardware. It provides basic functionality for developers and testers:

- Simulates virtual GPUs on CPU-only nodes
- Supports basic feature discovery and NVIDIA MIG support
- Generates Prometheus metrics for GPU monitoring
- Reduces hardware costs for testing environments

Use cases include:
- Testing GPU-dependent applications
- CI/CD pipeline testing
- Development environments
- Learning and experimentation

## ✨ Features

- Basic GPU topology simulation
- Prometheus metrics generation
- NVIDIA MIG simulation support
- Configurable GPU types and memory
- Basic nvidia-smi simulation

## 🏃 Quick Start

### Prerequisites

- Kubernetes cluster without NVIDIA GPU Operator
- Helm 3.x
- kubectl CLI tool

### 1. Label Your Nodes
