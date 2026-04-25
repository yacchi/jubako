# Jubako Design Docs

This directory contains durable design notes for Jubako.

The intent is different from `plans/`:

- `docs/design/` captures the current design model and the reasons behind it
- `plans/` captures exploratory work, drafts, and implementation planning

The Japanese files are the primary reference for the maintainer. English files are kept nearby so the same structure
is available for future external documentation.

## Documents

- [layer-model.md](layer-model.md): core abstractions and layer boundaries
- [materialization.md](materialization.md): merge, schema mapping, and origin tracking
- [write-path.md](write-path.md): `SetTo`, dirty tracking, and save behavior
- [runtime-and-watch.md](runtime-and-watch.md): resolved state, subscriptions, and watching
- [coordinated-layer.md](coordinated-layer.md): logical layers backed by multiple physical stores

## Reading Order

1. Start with [layer-model.md](layer-model.md)
2. Then read [materialization.md](materialization.md)
3. Read [write-path.md](write-path.md) and [runtime-and-watch.md](runtime-and-watch.md) depending on the area you are changing
4. Read [coordinated-layer.md](coordinated-layer.md) when working on multi-store projection or external secret flows
