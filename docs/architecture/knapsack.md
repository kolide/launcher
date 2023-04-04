# Knapsack

`Knapsack` is an object instantiated very early on in the launcher startup process, and contains an inventory of useful services which are used throughout launcher. Its purpose is to provide a convenient API to access commonly used resources and functionality, while minimizing function argument list bloat.

`Knapsack` is a superset of several interfaces, forming an API with a large surface area, resembling a Facade design pattern. `Knapsack` itself largely delegates operations and complexity to sub-components, as it's primary goal is to provide a simple API for any client within launcher to use easily and effectively.

### Mocking Knapsack

Many times, tests can instantiate a mock `Knapsack` and provide mock method implementations for only the methods that are invoked by the test in question. For example:

```
mockKnapsack := typesMocks.NewKnapsack(t)
mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval
mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
```

## Functionality

### Stores

`Knapsack` provides access to several `KVStore` objects which are used by various launcher components.

As an example, this is how you would access the launcher configuration data and get the `nodeKey`:

```
var k knapsack // Passed into your code as a dependency
key, err :=k.ConfigStore().Get([]byte(nodeKey))
```

### Flags

`Knapsack` provides direct access to the `Flags` interface, which allows clients to store and retrieve launcher flags.

As an example, this is how you would access the control server request interval:

```
var k knapsack // Passed into your code as a dependency
interval := k.ControlRequestInterval()
```