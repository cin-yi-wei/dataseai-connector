# DataseAI Connector GUI

## Linux runtime dependencies

The Linux GUI uses GTK/WebKitGTK through Wails.

Ubuntu 24.04+ / Debian 13+:

```bash
sudo apt-get update
sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0
```

Older Ubuntu releases that still ship WebKitGTK 4.0 need a GUI binary built
against 4.0. Current Linux release builds target WebKitGTK 4.1.

## Linux service permissions

Enter the agent token in the GUI. The GUI writes it to a temporary 0600 config
file and uses `pkexec` for service install/start/stop actions, so users should
not paste tokens into a terminal command.

## Live Development

To run in live development mode, run `wails dev` in the project directory. This will run a Vite development
server that will provide very fast hot reload of your frontend changes. If you want to develop in a browser
and have access to your Go methods, there is also a dev server that runs on http://localhost:34115. Connect
to this in your browser, and you can call your Go code from devtools.

## Building

To build a redistributable, production mode package, use:

```bash
make gui-build
```

On Linux this adds Wails' `webkit2_41` build tag so the resulting binary links
against `libwebkit2gtk-4.1.so.0`, which is available on current Ubuntu releases.
