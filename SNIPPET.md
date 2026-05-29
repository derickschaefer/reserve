# reserve Snippet System Redesign Proposal
## Target Release: v1.1.7 Foundation → v1.2 Ecosystem Expansion

Author: Derick Schaefer
Audience: Codex / implementation LLMs
Status: Approved Direction
Date: 2026-05-23

---

# Executive Summary

The original `reserve snippet` implementation stored snippets directly inside
`config.json` and imposed a hard limit of 10 snippets.

That design is now considered obsolete.

The snippet system is evolving from:

> "saved shell commands"

into:

> "portable, shareable, documented economic workflow packages"

The v1.1.7 goal is NOT to build the full ecosystem yet.
The goal is to lay a scalable architectural foundation that naturally grows into:

- local snippet libraries
- shared snippet collections
- parameterized workflows
- installable snippet packs
- future remote registries/APIs
- trusted/public/community ecosystems

---

# High-Level Design Direction

## Old Model

```json
{
  "snippets": {
    "foo": {
      "cmd": "reserve obs get CPIAUCSL ...",
      "desc": "..."
    }
  }
}
```

Problems:

- config.json becomes bloated
- impossible to scale cleanly
- no metadata/versioning/grouping
- no package semantics
- difficult future API integration
- no namespace/library model
- hard to share snippets between users

---

# New Architecture

## config.json only stores snippet system configuration

Example:

```json
{
  "snippet": {
    "home": "~/.reserve/snippets",
    "enabled": [
      "personal",
      "official"
    ]
  }
}
```

The actual snippet definitions live in filesystem libraries.

---

# Filesystem Layout

## Initial Recommended Layout

```text
~/.reserve/snippets/
  personal/
    snippets.yaml

  official/
    snippets.yaml

  ppi-pack/
    snippets.yaml
```

Alternative future-compatible layout:

```text
~/.reserve/snippets/
  personal.yaml
  official.yaml
  ppi-pack.yaml
```

Directory-based layout is preferred because it scales better later for:

- assets
- manifests
- README docs
- signatures
- version metadata
- examples

---

# Snippet Library File Format

YAML is preferred for local authoring readability.

JSON support may be added later.

---

# YAML Schema Example

```yaml
schema: reserve.snippets/v1

name: ppi-pack
title: Producer Price Index Snippets

description: >
  Useful PPI workflows for inflation and manufacturing analysis.

version: 0.1.0

author: Derick Schaefer

license: MIT

tags:
  - ppi
  - inflation
  - prices

snippets:

  pcu_annual_bar:

    title: Semiconductor PPI annual bar chart

    description: >
      Annual mean bar chart of semiconductor and electronic
      component producer prices.

    command: >
      reserve obs get PCU3344133441
      --start 2018-01-01
      --end 2026-05-01
      --format jsonl
      | reserve transform resample --freq annual --method mean
      | reserve chart bar

    tags:
      - ppi
      - semiconductor
      - chart

    series:
      - PCU3344133441
```

---

# Command Naming Direction

## Existing

```bash
reserve snippet set
reserve snippet get
reserve snippet list
reserve snippet delete
reserve snippet run
```

These remain valid.

---

# Future Naming Convention

Snippets become namespaced by library.

Example:

```bash
reserve snippet run ppi-pack/pcu_annual_bar
```

If globally unique:

```bash
reserve snippet run pcu_annual_bar
```

---

# v1.1.7 Scope

## Goals

- move snippet storage OUT of config.json
- remove artificial snippet count limits
- support filesystem-backed snippet libraries
- preserve current UX simplicity
- create forward compatibility with v1.2 package ecosystem

---

# v1.1.7 Required Features

## 1. Config-based snippet home

Add:

```json
{
  "snippet": {
    "home": "~/.reserve/snippets"
  }
}
```

Default path if unset:

```text
~/.reserve/snippets
```

---

## 2. Filesystem-backed snippet persistence

`snippet set` should now:

- load YAML library
- update snippet entry
- persist back to disk

NOT mutate config.json directly.

---

## 3. Remove snippet count limit

No hard-coded snippet limits.

The filesystem is now the scalability boundary.

---

## 4. Default library

If no library specified:

```bash
reserve snippet set foo ...
```

should target:

```text
personal/snippets.yaml
```

---

## 5. Library-aware resolution

Support:

```bash
reserve snippet run foo
```

Search order:

1. personal
2. enabled libraries
3. first unique match wins

If ambiguous:

```text
snippet name "foo" exists in:
  - personal/foo
  - official/foo

Please specify a library-qualified name.
```

---

# v1.2 Roadmap (NOT Required Yet)

The following are intentionally deferred.

---

# Future Feature: Parameterized Snippets

Example:

```yaml
command: >
  reserve obs get CPIAUCSL
  --start {{start}}
  --end {{end}}
  --format jsonl
  | reserve chart plot
```

Run:

```bash
reserve snippet run inflation_chart \
  start=2020-01-01 \
  end=2025-01-01
```

Potential implementation:

- Go text/template
- Mustache
- custom placeholder expansion

---

# Future Feature: Structured Pipelines

Current snippets are shell strings.

Future snippets may support a structured AST/pipeline model.

Example:

```yaml
pipeline:
  - cmd: obs get
    args:
      - CPIAUCSL
    flags:
      start: "{{start}}"
      format: jsonl

  - cmd: transform pct-change
    flags:
      period: 12

  - cmd: chart plot
```

Advantages:

- safer execution
- introspection
- validation
- LLM generation
- editor tooling
- API portability

This is NOT part of v1.1.7.

---

# Future Feature: Snippet Registries

Potential future commands:

```bash
reserve snippet search inflation --remote

reserve snippet install official/ppi-pack

reserve snippet update
```

Potential registry index:

```json
{
  "packages": [
    {
      "name": "ppi-pack",
      "version": "0.1.0",
      "url": "https://snippets.reserve.dev/ppi-pack.yaml",
      "sha256": "..."
    }
  ]
}
```

---

# Future Feature: Trust/Security Model

Because snippets execute shell commands, trust boundaries matter.

Potential future metadata:

```yaml
execution:
  shell: true
  network: true
  writes_files: false
```

Potential future UX:

```text
WARNING:
Snippet executes arbitrary shell commands.

Trust this library?
```

NOT required in v1.1.7.

---

# Design Philosophy

The snippet system should evolve into:

- reusable economic workflows
- portable analysis recipes
- versioned pipeline collections
- shareable research tooling
- community-distributed macro libraries

This is strategically important because it transforms reserve from:

> a CLI utility

into:

> an economic workflow platform

---

# Important Constraints

## v1.1.7 must remain simple

Avoid overengineering.

DO NOT build:

- registries
- networking
- signatures
- package servers
- OAuth
- permissions
- AST execution engines

Yet.

The only requirement right now is:

> filesystem-backed scalable snippet libraries.

---

# Immediate Implementation Recommendation

## Recommended Internal Types

```go
type SnippetLibrary struct {
    Schema      string              `yaml:"schema"`
    Name        string              `yaml:"name"`
    Title       string              `yaml:"title"`
    Description string              `yaml:"description"`
    Version     string              `yaml:"version"`
    Author      string              `yaml:"author"`
    License     string              `yaml:"license"`
    Tags        []string            `yaml:"tags"`
    Snippets    map[string]Snippet  `yaml:"snippets"`
}

type Snippet struct {
    Title       string   `yaml:"title"`
    Description string   `yaml:"description"`
    Command     string   `yaml:"command"`
    Tags        []string `yaml:"tags"`
    Series      []string `yaml:"series"`
}
```

---

# Recommended Initial Commands

## Required

```bash
reserve snippet set
reserve snippet get
reserve snippet list
reserve snippet delete
reserve snippet run
```

---

# Strongly Recommended

```bash
reserve snippet validate
reserve snippet export
```

---

# Recommended UX Examples

## Create

```bash
reserve snippet set pcu_annual_bar \
  --desc "Annual semiconductor PPI bar chart" \
  --cmd "reserve obs get ..."
```

---

## Run

```bash
reserve snippet run pcu_annual_bar
```

---

## Explicit library

```bash
reserve snippet run ppi-pack/pcu_annual_bar
```

---

## List

```bash
reserve snippet list

reserve snippet list --library ppi-pack
```

---

# Final Strategic Direction

Snippets are no longer:

> aliases stored in config

Snippets are becoming:

> composable economic workflow packages

That architectural distinction matters and should guide all future implementation.
