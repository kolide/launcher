# Control Server

## Functionality

The control server feature enables launcher to periodically query Kolide's SaaS app to receive
data updates for various subsystems of launcher. Each subsystem is a named component and has a hash
of it's data which can be used to support ETags. When the launcher's control service finds a new
update for a subsystem, it notifies a consumer registered to handle updates for the subsystem, then pings all subscribers for the subsystem.

The latest update for each subsystem is cached so the control server can avoid re-sending updates
previously sent to a launcher instance.


## Protocol

```mermaid
sequenceDiagram
    participant K2
    participant ControlService
    participant Consumer
    participant Subscriber

    loop On request interval
        ControlService->>K2: GET /api/v1/control
        K2-->>ControlService: Returns list of subsystems & hashes

        loop For each subsystem
            alt If cached update is still fresh
                ControlService->>ControlService: Skip to next subsystem
            else
                ControlService->>K2: GET /api/v1/control/{objectHash}
                K2-->>ControlService: Returns latest subsystem data
            end

            ControlService->>Consumer: Update(data)

            loop For each subscriber
                ControlService->>Subscriber: Ping()
            end

            ControlService->>ControlService: Cache update
        end
    end
```