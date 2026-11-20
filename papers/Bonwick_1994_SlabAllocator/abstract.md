# Abstract

## Original Text (Verbatim)

This paper presents a comprehensive design overview of the SunOS 5.4 kernel memory allocator. This allocator is based on a set of object-caching primitives that reduce the cost of allocating complex objects by retaining their state between uses. These same primitives prove equally effective for managing simple memory (e.g. data pages and temporary buffers) because they are space-efficient and fast. The allocator's object caches respond dynamically to global memory pressure, and employ an object-coloring scheme that improves the system's overall cache utilization and bus balance. The allocator also has several statistical and debugging features that can detect a wide range of problems throughout the system.

## Our Interpretation

The design targets repeated construction as well as memory allocation. Barn
should measure both costs and reclamation independently; the reported SunOS
results provide no Barn speedup estimate.
