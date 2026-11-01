# BlackShield Security Audit Report — `cashapp/hermit`

- **Scanned Commit:** `5db2c8b00761efce56cdcc4182e21e0ce689ac75`
- **Actionable Findings:** `1` (`0 Critical`, `1 High`, `1 Medium`, `0 Low`)
- **BlackShield Platform & Continuous Zero-Cost PR Protection:** [https://blackshield.chaplau.com](https://blackshield.chaplau.com)

## Detailed Findings & Remediation Guide

### 1. [HIGH] `go-cmdi-exec-combined` — go-cmdi-exec-combined: Command Execution via Dynamic exec.Command

- **Category:** `sast`
- **Location:** `util/run.go:67`
- **Details:** Go application security vulnerability or insecure coding pattern. at util/run.go:67
- **Recommended Fix:** Invoke the target binary directly with exec.Command(bin, args...) without sh -c shell evaluation.

