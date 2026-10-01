# phpscript code coverage

Statement coverage of the Go packages, one row per package, with the cognitive complexity and the line count each one carries. [phpscript-detail.md](phpscript-detail.md) is the same run per function.

Status is the coverage requirement: 80% of lines, or a cognitive complexity under 5 with any coverage at all. A function with complexity 0 has no branch to miss and is covered by being called.

## Packages

| Status | Package                | Coverage | Cognitive | Lines |
|--------|------------------------|----------|-----------|-------|
| ❌     | .                      | 9.17%    | 40        | 180   |
| ✅     | annotations            | 87.75%   | 152       | 680   |
| ❌     | cmd/phpscript/ast      | 0.00%    | 5         | 29    |
| ❌     | cmd/phpscript/fmt      | 0.00%    | 5         | 38    |
| ✅     | cmd/phpscript/helpdocs | 97.64%   | 38        | 145   |
| ✅     | cmd/phpscript/info     | 85.59%   | 52        | 179   |
| ✅     | cmd/phpscript/lint     | 83.05%   | 57        | 209   |
| ✅     | cmd/phpscript/list     | 52.94%   | 4         | 35    |
| ❌     | cmd/phpscript/run      | 5.37%    | 26        | 168   |
| ✅     | cmd/phpscript/server   | 83.39%   | 166       | 974   |
| ❌     | cmd/phpscript/test     | 77.45%   | 510       | 1812  |
| ❌     | cmd/phpscript/version  | 0.00%    | 31        | 79    |
| ✅     | config                 | 88.55%   | 103       | 407   |
| ✅     | flatstack              | 60.00%   | 0         | 14    |
| ✅     | flatstack/engine       | 82.75%   | 1253      | 2769  |
| ✅     | formatter              | 84.61%   | 310       | 1264  |
| ✅     | internal/apidoc        | 84.82%   | 373       | 989   |
| ✅     | internal/arrayi64      | 100.00%  | 6         | 18    |
| ✅     | internal/flags         | 90.65%   | 76        | 215   |
| ✅     | internal/phpval        | 94.25%   | 195       | 678   |
| ✅     | internal/table         | 100.00%  | 24        | 117   |
| ❌     | lint                   | 79.25%   | 307       | 981   |
| ❌     | list                   | 73.52%   | 136       | 427   |
| ✅     | model                  | 91.09%   | 329       | 986   |
| ✅     | parser                 | 88.38%   | 1061      | 3258  |
| ✅     | runner                 | 85.99%   | 2003      | 7496  |
| ✅     | runner/coverage        | 94.17%   | 134       | 482   |
| ✅     | runner/expr            | 80.63%   | 192       | 580   |
| ✅     | runner/mapmap          | 81.40%   | 94        | 296   |
| ❌     | scripts/list-apis      | 0.00%    | 1         | 13    |
| ✅     | stdlib                 | 95.83%   | 5         | 58    |
| ✅     | stdlib/compat          | 87.89%   | 184       | 756   |
| ✅     | stdlib/core            | 88.83%   | 1101      | 4368  |
| ❌     | stdlib/crypto          | 78.00%   | 69        | 333   |
| ✅     | stdlib/database        | 87.75%   | 111       | 648   |
| ✅     | stdlib/files           | 87.82%   | 289       | 931   |
| ✅     | stdlib/gd              | 82.35%   | 154       | 535   |
| ❌     | stdlib/http            | 79.00%   | 112       | 675   |
| ✅     | stdlib/info            | 66.67%   | 0         | 9     |
| ✅     | stdlib/internals       | 100.00%  | 0         | 16    |
| ✅     | stdlib/logger          | 100.00%  | 9         | 53    |
| ❌     | stdlib/mail            | 58.71%   | 52        | 283   |
| ✅     | stdlib/pexec           | 92.86%   | 41        | 210   |
| ✅     | stdlib/regexp          | 100.00%  | 0         | 13    |
| ❌     | stdlib/session         | 72.48%   | 72        | 316   |
| ✅     | stdlib/shared          | 98.00%   | 57        | 165   |
| ✅     | stdlib/span            | 100.00%  | 0         | 9     |
| ✅     | stdlib/time            | 94.46%   | 5         | 135   |
| ✅     | telemetry              | 94.83%   | 12        | 119   |
| ✅     | tests                  | 84.70%   | 314       | 1544  |
