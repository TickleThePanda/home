---
paths:
  - "deploy/internal/homepage/**"
---

# Homepage

An application launcher first, not a monitoring dashboard. These are
principles, not a fixed layout.

- Apps the owner opens day to day (Audiobookshelf, Home Assistant, GeoPulse,
  Label Studio) go in Applications: large cards at the top.
- Infrastructure, networking and IAM go in Administration: one compact area
  below, smaller cards, subtle status.
- No individual nodes or VMs (that's Grafana/Headlamp), no search bar, and
  low-priority metrics only at the edge.
- `custom.css` targets groups by render order, which follows `settings.yaml`'s
  `layout:` keys. Keep that block to exactly the groups the CSS expects, in
  the same order.
