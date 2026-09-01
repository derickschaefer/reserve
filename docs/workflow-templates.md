# Workflow Templates

This file provides copyable starting points for RESERVE workflow YAML.

The workflow model supports two common styles:

- static workflows with no runtime inputs
- dynamic workflows with positional runtime inputs

Use `@1`, `@2`, and so on only for dynamic workflows. Leave `contract` empty for
static workflows.

---

## Static Workflow Template

Use this when the workflow has no runtime inputs and should run the same way
every time.

```yaml
workflow:
  title: Inflation Dashboard
  summary: Repeatable CPI analysis

  difficulty: Beginner
  estimated_runtime: 5 seconds

  categories:
    - inflation

  concepts:
    - CPI
    - FRED
    - Time Series

  outputs:
    - chart
    - json

  requires_network: true
  reserve_version: ">=1.2"

  author: Derick Schaefer
  documentation: README.md

  pipeline:
    - ./reserve obs get CPIAUCSL --start 2020-01-01 --end 2024-12-31 --format jsonl
    - ./reserve analyze summary
```

Static workflows:

- declare no `contract`
- use fixed command arguments in `pipeline`
- are best when the analysis always uses the same parameters

---

## Dynamic Workflow Template

Use this when the workflow should accept runtime inputs from the command line.

```yaml
workflow:
  title: GDP Summary
  summary: Summarize GDP over a date range

  difficulty: Beginner
  estimated_runtime: 5 seconds

  categories:
    - gdp

  concepts:
    - GDP
    - FRED
    - Time Series

  outputs:
    - json
    - chart

  requires_network: true
  reserve_version: ">=1.2"

  author: Derick Schaefer
  documentation: README.md

  contract:
    - label: start date
      format: YYYY-MM-DD
      sample: 2020-01-01
      description: First date in the requested range
    - label: end date
      format: YYYY-MM-DD
      sample: 2024-12-31
      description: Final date in the requested range

  pipeline:
    - ./reserve obs get GDP --start @1 --end @2 --format jsonl
    - ./reserve analyze summary
```

Dynamic workflows:

- declare ordered runtime inputs in `contract`
- reference those inputs in `pipeline` with `@1`, `@2`, and so on
- use `workflow contract <NAME>` to print the runtime contract
- use `workflow render <NAME> <value1> <value2>` to produce one copy-ready pipeline command

---

## Notes For Authors

- Use `title` and `summary` to explain what the workflow does.
- Use `categories` for broad topical grouping.
- Use `concepts` for display-friendly keywords.
- Use `outputs` to describe the result types.
- Use `documentation` to point to a README or design note. In a namespaced
  collection, the path is relative to the collection directory.
- `workflow create <REPOSITORY/COLLECTION>` scaffolds the collection's
  `README.md` and never overwrites existing documentation.
- Keep each `pipeline` item to one clear, deterministic pipe stage.
- `workflow render` joins stages with ` | ` but never executes the resulting command.

---

## Suggested Commands

```bash
./reserve workflow create official/inflation
./reserve workflow create official/inflation/gdp-summary
./reserve workflow validate official/inflation/gdp-summary
./reserve workflow contract GDP-Summary
./reserve workflow render GDP-Summary 2020-01-01 2024-12-31
```
