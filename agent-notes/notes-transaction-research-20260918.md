# Transaction research checkpoint 2026-09-18
- Read full ICPP2008 inevitability and DSN2011 scheduling papers; reading remaining PPoPP2009 evaluation.
- PPoPP2009 section 3.4 disclaims proven starvation/livelock freedom for its Karma composition despite abstract shorthand.
- Promotion must protect prior reads and validate before external effects; a fair global gate alone does not enable disjoint commits.
- Next: inspect pinned TinySTM code and MVCC comparison, then write owned research report. No runtime edits.
