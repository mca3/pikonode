# pikonode client-side software

*This is a part of the Pikonet project.*
[Go see the server side.](https://github.com/mca3/pikorv)

This repository contains `pikonodectl`, a tool for managing Pikonet networks,
and `pikonoded`, a tool for connecting to them.

**Note**: Only Linux hosts with an in-kernel implementation are supported at
this time.
There is no documentation on how to set this up, you're on your own.

## Windows compatibility

pikonode has been **lightly** tested on Windows.
It is known to **not** work on Windows 7 unless you use WireGuardNT.
I have not tried on Windows 10, yet.

Additionally:

- Broadcast discovery does not work, but it will whenever I switch over to using
  multicast discovery
- The network adapter is recreated on every execution
- Likely more I have not figured out yet
