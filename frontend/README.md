## Frontends interface

Mailbox provides ability for user to use any kind of UI they want, including
but not limited to CLI.

"Frontend" implementation links "core library" (usually statically) and uses it
to interact with everything (configuration, messages, account data).

### Official frontends

- CLI utility (`cli` sub-package)
- Full-blown GUI application (`gui` sub-package)

### Other sub-packages

- Core API library (that one that should be linked in)  (`api` sub-package)

  Covered by API stability guarantees.
