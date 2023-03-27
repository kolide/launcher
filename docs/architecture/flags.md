# Knapsack.Flags

The `Flags` interface provides a simple API for storing and retrieving launcher flags at runtime. Currently, flags are of types `bool`, `int64` and `string`.

Launcher flags are identified by a `FlagKey` and can be specified through various means:

- Default values, which are used when no other value has been provided.
- Command line values, which can be fed into launcher via command line options, config files, or environmental variables.
- Control server updates, which are ingested by the control server client and stored in a key-value store.
- Temporary overrides, where a client can request the `Flags` interface to override the current value with a different value for a duration of time.

Launcher flag values can also have constraints defined, which provide safeguards to prevent unreasonable values being used.

## Retrieving Flags

```mermaid
flowchart TB
    Client[Client]
    Default[Use Default Value]
    Sanitize[Sanitize Integer Flags]
    Override{Is flag temporarily overridden?}
    Store{Has control server provided a value?}
    CmdLine{Was a command line flag provided?}

    Client -->|"Knapsack.Flags.DesktopEnabled()"| Override
    Override -->|Yes| Sanitize
    Override -->|No| Store

    Store -->|Yes| Sanitize
    Store -->|No| CmdLine

    CmdLine -->|Yes| Sanitize
    CmdLine -->|No| Default

    Default --> Sanitize

    Sanitize -.->|Return value to Client| Client

```

## Storing Flags

```mermaid
flowchart TB
    Client[Client]
    Store[Store Flag]
    Notify[Notify Observers]
    Error{Did an error occur?}

    Client -->|"Knapsack.Flags.SetDesktopEnabled(enabled)"|Store

    Store --> Error
    Error -->|No| Notify
    Error -.->|Yes, return err to Client| Client

    Notify -.->|Return value to Client| Client
```

## Storing Temporary Overrides

```mermaid
flowchart TB
    Client[Client]
    Store[Store Override Flag]
    Notify[Notify Observers]
    Override{Is value changing?}
    Store[Async Wait for Override Expiration]
    CmdLine[Clear Override Flag]

    Client -->|"Knapsack.Flags.SetTemporaryOverride"|Override

    Override -->|Yes| Store
    Override -.->|No| Client

    Store -.-> Store
    Store --> Notify

    Store -->CmdLine
    CmdLine -->Notify

    Notify -.->|Return err to Client| Client
```