---
tags: [memory-management, allocation]
---
Bonwick describes typed object caching, slab allocation, pressure-driven reclamation and cache coloring in the SunOS 5.4 kernel. Measurements separate allocation from construction costs and compare both isolated allocator performance and application workloads. For Barn, this motivates measuring construction and cleanup separately while preserving object identity and existing shared property storage.
