# Code conventions

This document outlines key code conventions followed in this project.

## Go

1. **Avoid utility packages** <br />
   Do not use generic utility package names like `base`, `util`, or `common`. For more context, see [Dave Cheney's post explaining why](https://dave.cheney.net/2019/01/08/avoid-package-names-like-base-util-or-common).
2. **Error wrapping** <br />
   Errors should **always** be wrapped with `fmt.Errorf` with the `%w` directive. The wrapped message must provide enough context to help identify the error’s source at a glance. Avoid prefixes like "failed to" or "error" in messages, as they add no meaningful context.
   - **Bad Example** <br />
     A generic message that doesn’t provide specific context:
     ```go
     if _, err := os.Create(configFileName); err != nil {
       return fmt.Errorf("failed to create file: %v", err)
     }
     ```
   - **Good Example** <br />
     A more specific message that clarifies the error’s context:
     ```go
     if _, err := os.Create(configFileName); err != nil {
       return fmt.Errorf("create config file: %w", err)
     }
     ```
3. **Struct constructors** <br />
   Struct constructors are specifically created for use within `app.go` and `main.go`. They may initialize dependency fields with meaningful defaults and use static variables from other packages. An implication of this is that **public tests** should use constructors to initialize structs, and **private tests** may create a struct manually.

### See also

- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
