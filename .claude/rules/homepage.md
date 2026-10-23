---
paths:
  - "deploy/internal/homepage/**"
---

# Homepage

An application launcher first, not a monitoring dashboard. It answers "what
do I want to open?" before "what am I running?". These are principles, not
a fixed layout.

- User-facing apps (the things opened day to day: Audiobookshelf, Home
  Assistant, GeoPulse, Label Studio) get the strongest priority: large
  cards, generous spacing, top of the page.
- Infrastructure, networking and IAM services stay reachable but secondary:
  one compact area below, smaller cards, subtle status indicators.
- Don't surface operational detail just because Homepage can. No individual
  nodes or VMs; that's Grafana/Headlamp's job. Low-priority info such as
  cluster metrics goes unobtrusively at the edge.
- Drop elements that don't earn their space (e.g. no search bar).
- For a new service, decide whether the owner opens it directly
  (Applications group) or it supports the system (Administration group),
  and style it accordingly.
- `custom.css` targets groups by render order within `#layout-groups`,
  since Homepage exposes no group-name hook. That order follows
  `settings.yaml`'s `layout:` keys, so keep that block to exactly the groups
  the CSS expects, in the same order.
