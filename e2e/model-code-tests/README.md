# Generated Go checks

Two small tasks compare returned code rather than just accepting an HTTP 200 response. They are functional examples, not a coding benchmark or proof of model equivalence.

- `FirstUnique`: nil/empty input, duplicates, order, empty strings, Unicode and unchanged input.
- `ParseRetryAfter`: decimal seconds, whitespace, HTTP dates relative to the supplied clock, past dates, invalid/signed values and duration overflow, including positive wraparound.

Review the returned Go source, place it in an empty directory as `candidate.go`, copy the matching fixture there as `candidate_test.go`, then run:

```bash
GO111MODULE=off GOTOOLCHAIN=local go test -count=1 -timeout=10s
```

The live comparison used Go 1.24.4. On failure, each model received its original answer and the compiler/test output for one correction attempt. Initial answers and corrections remain separate in the [results](../results/single-t4).
