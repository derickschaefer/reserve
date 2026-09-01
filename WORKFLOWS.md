# WORKFLOWS.md

Status: Accepted Architecture
Applies Beginning With: RESERVE 1.2
Author: Derick Schaefer
Audience: RESERVE Developers, Contributors, Codex
Last Updated: TBD

---

# 1. Introduction

The RESERVE Workflow System transforms RESERVE from a command-line utility into a
platform for creating, sharing, discovering, and rendering reproducible
macroeconomic analyses, with execution intentionally deferred until a secure,
cross-platform model is established.

A workflow is more than a saved command.

A workflow represents a repeatable economic analysis that can be inspected and
implemented by any RESERVE user regardless of experience level.

Examples include:

- Inflation dashboards
- GDP trend analyses
- Yield curve studies
- Labor market reports
- Classroom exercises
- Research methodologies
- Policy analysis pipelines

Workflows are intended to become one of RESERVE's defining capabilities.

---

# 2. Vision

The long-term vision is to build the largest collection of reusable,
reproducible macroeconomic workflows available anywhere.

RESERVE should become known not only as a command line tool, but as an ecosystem
of economic knowledge.

Users should be able to discover analyses written by:

- economists
- researchers
- educators
- students
- analysts
- government agencies
- universities
- the RESERVE project

Every workflow should answer one question:

> "How do I perform this economic analysis?"

The workflow captures the methodology.

RESERVE 1.2 renders the methodology for an operator or agent to review and
implement. A later release may perform execution once its semantics are secure
and cross-platform.

---

# 3. Design Philosophy

The workflow system is guided by several principles.

## 3.1 Workflows Are Content

A workflow is not configuration.

A workflow is content.

Content deserves:

- titles
- descriptions
- documentation
- authors
- versions
- licensing
- metadata
- discoverability

This philosophy distinguishes workflows from aliases or shell scripts.

---

## 3.2 Local First

Every workflow is stored, validated, and rendered locally.

Internet connectivity should only be required for:

- searching repositories
- installing workflows
- publishing workflows
- updating repositories

Rendering does not depend on remote infrastructure. Any future execution engine
must preserve this local-first boundary.

---

## 3.3 Human Readable

Workflow definitions must remain readable without specialized tooling.

YAML is preferred because it supports:

- source control
- manual editing
- documentation
- AI generation
- educational readability

---

## 3.4 Reproducible Research

A workflow should produce consistent results when executed against the same data.

Researchers should be able to cite a workflow by name and version.

Example:

    inflation-dashboard v1.2.0

rather than attempting to describe an entire shell pipeline.

---

## 3.5 Progressive Complexity

Beginning users should be able to install, inspect, and render workflows without
writing any YAML.

Advanced users may create and publish their own workflows.

Expert users may eventually build complete workflow collections.

---

## 3.6 Future Without Lock-In

RESERVE 1.2 represents pipelines as ordered text stages but does not execute
them. Operators and agents can inspect the resolved stages and decide how to
implement them.

Future versions of RESERVE may adopt a secure, cross-platform execution model.

The workflow specification must remain stable regardless of execution engine.

The workflow is the product.

The execution engine is an implementation detail.

---

# 4. Workflow Taxonomy

The workflow ecosystem is organized into three durable layers:

- repository
- collection
- workflow

Repositories define trust, ownership, and distribution boundaries.

Collections define topical bundles and packaging boundaries.

Workflows define the executable analysis itself.

### 4.1 Repository Types

Repositories are the top-level namespace and the main trust boundary.

The current design intentionally supports these repository types:

- `official`
- `community`
- `education`
- `personal`
- `private`

#### official

Owned by the RESERVE project.

Source of truth:

- the RESERVE core maintainers
- official releases or official repository sync

Use case:

- canonical workflows
- polished examples
- stable reference implementations

#### community

Owned by the broader contributor ecosystem.

Source of truth:

- public community repositories
- contributor-maintained packages
- curated community collections

Use case:

- useful workflows that are not project-canonical
- niche or exploratory analyses
- shared community examples

#### education

Owned by instructors, curriculum authors, and education-focused maintainers.

Source of truth:

- lesson repositories
- classroom bundles
- course-managed workflow collections

Use case:

- guided exercises
- teaching material
- reproducible classroom demonstrations

#### personal

Owned by the individual user.

Source of truth:

- the local filesystem
- the user’s private workflow store

Use case:

- local experimentation
- private notebooks-to-workflow conversions
- one-off or disposable analyses

#### private

Owned by a company, lab, consulting team, or other internal organization.

Source of truth:

- internal repositories
- private package feeds
- controlled org-specific workflow stores

Use case:

- internal methodology
- proprietary analysis
- team-shared workflows that are not public

### 4.2 Collections

A collection is a topical bundle inside a repository.

Examples:

- `official/inflation`
- `official/gdp`
- `education/introduction-to-economics`
- `personal/my-analysis`

Collections are created and maintained by the repository owner or curator.

Collections provide:

- packaging
- documentation
- versioning
- grouping
- install boundaries

### 4.3 Workflows

A workflow is the execution unit.

Workflows may be:

- zero-slot, hard-coded workflows
- positional dynamic workflows with runtime contract slots

Workflows are the things users run.

### 4.4 Runtime Contracts

Runtime contracts declare how positional inputs map into the workflow.

Examples:

- `@1` = first input
- `@2` = second input
- `@X` = Xth input

Contracts may include:

- label
- format
- sample
- description

# 5. User Personas

The workflow system is designed around four primary audiences.

---

## 5.1 Consumer

Consumers inspect contracts and render workflows.

Typical commands:

    reserve workflow search inflation

    reserve workflow install inflation-dashboard

    reserve workflow contract inflation-dashboard

    reserve workflow render inflation-dashboard

Consumers rarely edit workflow files.

---

## 5.2 Author

Authors create workflows.

Typical commands:

    reserve workflow create

    reserve workflow edit

    reserve workflow validate

Authors publish reusable analyses.

---

## 5.3 Educator

Educators assemble collections of workflows into lessons.

Examples:

- Introduction to Economics
- Inflation
- Monetary Policy
- Labor Markets
- GDP

Students render and study workflows while learning economic concepts.

---

## 5.4 Organization

Organizations curate private workflow collections.

Examples include:

- universities
- research firms
- consulting companies
- investment firms
- government agencies

Organizations may publish internal workflow repositories.

---

# 6. Product Goals

The workflow system should allow users to:

- discover workflows
- install workflows
- organize workflows
- render workflows for review and implementation
- learn from workflows
- share workflows
- publish workflows

The workflow system should NOT require users to understand YAML.

---

# 7. Workflow Lifecycle

Every workflow progresses through a common lifecycle.

Create

↓

Validate

↓

Execute

↓

Share

↓

Publish

↓

Install

↓

Update

↓

Retire

The lifecycle applies equally to official and community workflows.

---

# 8. Workflow Collections

Individual workflows belong to collections.

A collection represents a related group of analyses.

Examples:

Federal Reserve Toolkit

contains

- Taylor Rule
- Yield Curve
- Balance Sheet
- Fed Funds

Inflation Toolkit

contains

- CPI Dashboard
- PPI Dashboard
- Inflation Persistence
- Inflation Expectations

Educational Curriculum

contains

- Lesson 1
- Lesson 2
- Lesson 3

Collections exist for organization and distribution.

Users primarily interact with workflows.

On disk, collections and workflows are stored under a repository namespace and a collection namespace:

```text
~/.reserve/workflows/
  <repository>/
    <collection>/
      collection.yaml
      README.md
      workflows/
        <workflow>.yaml
```

For namespaced workflows, `documentation` paths are relative to the collection
directory. Creating a collection scaffolds `README.md`; RESERVE preserves that
file on subsequent authoring operations so it can be maintained and distributed
as a normal repository artifact.

The local `personal` repository is the default starting point for user-authored workflows.

Repository, collection, and workflow identifiers may use letters, numbers,
dots, underscores, and hyphens, and must begin with a letter or number.
Repository-style references are confined to the workflow store, including when
existing directories contain symlinks. Explicit YAML file paths remain
available for local authoring outside that namespace.

Workflow documents use strict YAML decoding: unknown fields and multiple YAML
documents are rejected. A workflow may declare one `reserve_version`
comparison (`>=`, `>`, `<=`, `<`, `=`, `==`, or an exact version), which is
enforced during validation, rendering, and contract inspection.

---

# 9. Repository Model

Workflow collections are distributed through repositories.

Examples:

Official Repository

Community Repository

Education Repository

Personal Repository

Private Repository

The repository model allows identical CLI commands regardless of where
workflows originate.

Example:

    reserve workflow search inflation

may return results from multiple repositories.

Repositories are an implementation detail.

The user experience remains consistent.

The repository name is still important because it encodes the trust boundary and the distribution source.

---

# 10. CLI Design

The workflow system adopts a noun-verb command structure.

Examples:

    reserve workflow create

    reserve workflow show

    reserve workflow list

    reserve workflow edit

    reserve workflow remove

    reserve workflow render

    reserve workflow contract

    reserve workflow search

    reserve workflow install

    reserve workflow update

    reserve workflow publish

    reserve workflow validate

These commands naturally separate into two categories.

Local operations

- create
- edit
- list
- show
- remove
- render
- run
- validate

Repository operations

- search
- install
- update
- publish

This distinction should remain clear throughout future development.

---

# 11. Architectural Roadmap

The workflow ecosystem will evolve incrementally.

Version 1.2 establishes the local workflow foundation:

- repository namespaces
- collection manifests
- workflow documents
- validation and inspection
- edit and remove operations
- zero-slot and positional runtime contracts
- local, non-executing pipeline rendering

Pipeline execution is explicitly deferred. A later release may add it only after
RESERVE defines secure, deterministic, and cross-platform execution semantics.

Future releases introduce repository features.

No release should require redesigning previous architecture.

Each release should extend the ecosystem without breaking workflow definitions.

The architecture described in this document is intended to support many years of
future development.

---

# End of Part 1
