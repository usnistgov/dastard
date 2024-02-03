# Interting an arbitrary subset of µMUX (Abaco) channels

In February 2024, for the HEATES experiment at J-PARC, we needed the ability in software to invert the sign of an arbitrary subset of channels ([issue #330](https://github.com/usnistgov/dastard/issues/330)).

This is now available in Dastard through the `AbacoUnwrapOptions` configuration object, specifically the `InvertChan` slice. However! We do not plan to allow these values to be set from the Dastard Commander GUI. That would add complexity to the typical case and risk people setting values in an unwanted way. How can you set channels to be in this list?

## Method 1: change the Dastard configuration file

Look in `~/.dastard/config.yaml` for the abaco configuration. For instance:

```
abaco:
    activecards: []
    availablecards: []
    hostportudp: []
    abacounwrapoptions:
        rescaleraw: false
        unwrap: true
        bias: false
        resetafter: 20000
        pulsesign: 0
        invertchan: []
```

While Dastard is NOT running, you can change this configuration file and expect the changes to persist, unless modified by commands from the GUI. With Dastard stopped, find the `invertchan: []` entry, and fill the square brackets with a comma-separated list of channels that you want inverted.

This change should be preserved across runs of Dastard.

## Method 2: send an RPC request to a running Dastard.

I'm still working on a minimal python script that will connect to Dastard and make the change to the configuration...