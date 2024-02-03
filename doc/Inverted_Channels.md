# Interting an arbitrary subset of µMUX (Abaco) channels

In February 2024, for the HEATES experiment at J-PARC, we needed the ability in software to invert the sign of an arbitrary subset of channels ([issue #330](https://github.com/usnistgov/dastard/issues/330)).

This is now available in Dastard through the `AbacoUnwrapOptions` configuration object, specifically the `InvertChan` slice. How can you set channels to be in this list?

## Method 1: use the dastard-commander GUI

In the main window, under the Data Sources tab, if you select Abaco µMUX as the source type, you can find a large text edit box labeled Inverted Channels. This box is generally disabled (cannot be edited). If you want to add or remove channels from the inverted-channels list, you first need to enable the box. You can find the control in the Expert menu. Select the menu item `Change Inverted Chans` to enable editing in the Inverted Channels text box.

When you Start Data for an Abaco source, this list is "normalized":
* Words are split by whitespace and/or commas.
* Words are converted to integers (or ignored when they cannot be).
* Duplicates are removed.
* The numbers are sorted.

Then the normalized list is sent to the Dastard server (and used to change the entries in the text edit box). At this, the `Change Inverted Chans` expert menu item becomes un-checked. This is intended as a way to make it difficult to change the list of inverted channels (because it will not need frequent changes).

## Method 2: send an RPC request to a running Dastard.

I'm still working on a minimal python script that will connect to Dastard and make the change to the configuration...
