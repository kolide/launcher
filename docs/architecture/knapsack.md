# Knapsack

`Knapsack` is an object instantiated very early on in the launcher startup process, and contains an inventory of useful services which are used throughout launcher. Its purpose is to provide a convenient API to access commonly used resources and functionality, while minimizing function argument list bloat.

## Functionality

### Storage

`Knapsack` provides access to several `KVStore` objects which are used by various launcher components.

As an example, this is how you would access the launcher configuration data and get the `nodeKey`:

```
var k knapsack // Passed into your code as a dependency
key, err :=k.ConfigStore().Get([]byte(nodeKey))
```

### Flags

`Knapsack` provides access to the `Flags` interface, which allows clients to store and retrieve launcher flags.

As an example, this is how you would access the control server request interval:

```
var k knapsack // Passed into your code as a dependency
interval := k.ControlRequestInterval()
```