# Code conventions

This document outlines key code conventions followed in this project.

## Go

1. **Avoid utility packages** <br />
   Do not use generic utility package names like `base`, `util`, or `common`. For more context, see [Dave Cheney's post explaining why](https://dave.cheney.net/2019/01/08/avoid-package-names-like-base-util-or-common).
2. **Error wrapping** <br />
   - To add context to an error, wrap it with `fmt.Errorf` and `%w`. Name the operation that failed, specifically enough to identify the source at a glance, and without prefixes like "failed to" or "error": `fmt.Errorf("create config file: %w", err)`, not `fmt.Errorf("failed to create file: %v", err)`.
   - Return an error unwrapped only when its message already identifies the source.
   - Use `%v` only when callers must not match the inner error with `errors.Is` or `errors.As`, and say why in a comment.
3. **Struct constructors** <br />
   - An exported constructor (`NewClient`) is the production entry point. It supplies real dependencies and defaults, such as the system clock or embedded assets.
   - When tests need to replace those, move the initialisation into an unexported constructor of the same name (`newClient`) that takes every dependency as a parameter, and call it from the exported one.
   - In same-package tests, use the unexported constructor, or build the struct directly if it needs no initialisation. In an external test package (`foo_test`), use the exported constructor.
4. **Function order** <br />
   Follow the stepdown rule: place a function above the functions it calls, so that a file reads top-down and helpers sit below their callers. Where call order does not decide, put exported functions first. Keep the methods of one type together.
5. **Doc comments** <br />
   Write doc comments in the form that [Go Doc Comments](https://go.dev/doc/comment) describes, with one exception - an exported name does not need a doc comment if the comment would only restate the name.

### Tests

1. **Layout** <br />
   - Put cases that run the same code on different data in a table.
   - Give each case that needs its own setup or assertions a top-level test function.
   - Use hand-written `t.Run` blocks only for cases that share setup built in the parent test. Do not nest them.
2. **Naming** <br />
   - Name a top-level test after the behaviour it checks, without underscores: `TestCacheOnlyMissNeverDials`.
   - Name subtests with short lower-case phrases: `"equal versions"`.
   - In a table, use `tests` for the slice, `tt` for the loop variable, a leading `name` field, and a `want` prefix on fields that hold expected values.
3. **Failure messages** <br />
   Put the actual value before the wanted one: `isNewer(%q) = %v, want %v` or `got %d requests, want 0`. Do not use "expected".
4. **Parallelism** <br />
   Call `t.Parallel()` first in every test and subtest, unless the test or one of its subtests changes process-wide state (`t.Setenv`, package-level variables).
5. **Assertions** <br />
   Use the standard library only.
6. **Files** <br />
   Name test files `<name>_test.go`, with benchmarks in `<name>_benchmark_test.go` and no other suffixes. Put helpers and fakes below the tests.

### See also

- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
