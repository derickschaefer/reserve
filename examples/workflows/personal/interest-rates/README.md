# Interest Rates Workflow Collection

This collection contains workflows for examining Treasury yields and related
market signals.

## What Moved the 10-Year?

`ten-year-treasury-drivers` compares six daily FRED series over one requested
date range:

| Role | Series | Interpretation |
|---|---|---|
| Nominal 10-year yield | `DGS10` | The market yield being investigated |
| Real-yield component | `DFII10` | The inflation-indexed 10-year Treasury yield |
| Breakeven inflation | `T10YIE` | Market-implied average inflation over approximately ten years |
| Near-term policy proxy | `DGS2` | A market signal for the expected path of shorter-term rates |
| Yield-curve slope | `T10Y2Y` | The 10-year yield minus the 2-year yield |
| Model-estimated term premium | `THREEFYTP10` | Estimated compensation for holding duration risk |

The workflow uses two complementary analytical lenses:

1. `DGS10` is approximately `DFII10 + T10YIE`. Because FRED derives the
   breakeven series from the nominal and inflation-indexed yields, these are not
   independent explanatory variables.
2. Long-term yields can also be viewed as the expected path of short-term rates
   plus a term premium. `DGS2`, `T10Y2Y`, and `THREEFYTP10` provide market and
   model context for that lens.

The output is descriptive, not a causal attribution model. Series release
calendars differ, so observation counts and final available dates may differ
within the requested window.
