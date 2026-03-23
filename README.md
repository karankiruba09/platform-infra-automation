# Platform Infra Automation

## Description
This repository contains reusable tooling, templates, and automation patterns used to operate and scale production-grade kubernetes and virtualized infrastructure.

## Scope
- Kubernetes cluster lifecycle management
- Platform upgrades and change automation
- Infrastructure reliability and post-check validation
- Storage and networking operational scripts

## Principles
- Automation-first operations
- Safe, repeatable upgrades
- Clear separation of platform and workload concerns
- Reliability and observability driven

## tca-tkg-precheck
Single-run pre-upgrade health report CLI for VMware Telco Cloud Automation (TCA) and Tanzu Kubernetes Grid (TKG) clusters.

Location: `kubernetes/tca-tkg-precheck`

Docs: `kubernetes/tca-tkg-precheck/README.md`

## Avi VirtualService Template Toolkit
Compare two existing Avi Virtual Services to generate a reusable common template plus a variable-fill catalog, then deploy new VS instances with only the remaining values.

Location: `networking/avi-virtualservice-templater-ansible`

Docs: `networking/avi-virtualservice-templater-ansible/README.md`
