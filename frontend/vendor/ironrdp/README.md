# Vendored IronRDP web client

Built output of [IronRDP](https://github.com/Devolutions/IronRDP), MIT or Apache-2.0.

- Source: head of [PR #1977](https://github.com/Devolutions/IronRDP/pull/1977), commit `af23bd5c427eb890686852cbd7a12f78681abbad`.
- Why not the npm packages: they do not advertise the Graphics Pipeline, which GNOME Remote Desktop requires.
- Rebuild: `scripts/build-ironrdp.sh`. CI does not need Rust, because the build output is committed here.
- Do not edit these files by hand.
