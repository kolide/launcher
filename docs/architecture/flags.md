# Knapsack.FlagsA

The `Flags` interface provides a simple API for storing and retrieving launcher flags at runtime. Flags are of type `bool`, `int64` and `string`.

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

    Client -->|Knapsack.Flags.SetOverride|Override

    Override -->|Yes| Store
    Override -.->|No| Client

    Store -.-> Store
    Store --> Notify

    Store -->CmdLine
    CmdLine -->Notify

    Notify -.->|Return err to Client| Client
```