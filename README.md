# pikonode client-side software

*This is a part of the Pikonet project.*
[Go see the server side.](https://github.com/mca3/pikorv)

This repository contains `pikonodectl`, a tool for managing Pikonet networks,
and `pikonoded`, a tool for connecting to them.

**Note**: Only Windows and Linux hosts are supported currently. It is assumed
you want the in-kernel implementation.
There is no documentation on how to set this up, you're on your own.

## Windows compatibility

pikonode has been **lightly** tested on Windows.
It is known to **not** work on Windows 7 unless you use WireGuardNT.
I have not tried on Windows 10, yet.

Additionally:

- The network adapter is recreated on every execution
- Probably much, much more. That's just all I know.
