# RotomNG (MITM Connection Manager)

RotomNG is a service that connects applications to MITM workers.

A single `rotom-ng` is the whole product: it serves its own web UI alongside its
API, and most deployments need nothing else. If you run several, there is an
optional second service that puts one UI in front of all of them.

## Documentation

- [Getting Started](docs/RotomNG-Starting.md) - Configuration and setup guide
- [Multi-instance admin UI](docs/RotomNG-UI-Server.md) - **Optional.** One web UI for several rotom-ng instances; skip it if you run one
- [HTTP API Reference](docs/RotomNG-API.md) - REST API endpoints for managing devices, controllers, jobs, and system configuration
- [Migration Guide (OG to NG)](docs/RotomNG-Vs-OG.md) - Differences between Rotom OG and Rotom NG for those upgrading
