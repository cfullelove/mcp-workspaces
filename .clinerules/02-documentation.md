### Rule: Keep Documentation in Sync with Code

**Intent:**
Ensure that all code changes are reflected in project documentation, specifically `README.md` and `TECHNICAL_OVERVIEW.md`, to maintain clarity and consistency.

**Guideline:**

* When making **any code update** (features, fixes, refactors, dependencies, configuration changes), evaluate whether the change requires an update to `README.md` and/or `TECHNICAL_OVERVIEW.md`.
* If the documentation already reflects the current state, explicitly confirm no update is required.
* If updates are needed, edit the relevant sections of the documentation **in the same commit/PR as the code change**.
* Prioritize:

  * `README.md` → update instructions, usage examples, prerequisites, and quick-start info.
  * `TECHNICAL_OVERVIEW.md` → update architecture, design rationale, component interactions, dependencies, or data flows.
* If uncertain, default to updating documentation rather than leaving it stale.
* Cross-reference code changes and documentation in commit messages for traceability.

**Example:**

* Adding a new CLI flag → update usage examples in `README.md`.
* Refactoring a core service → update architecture notes in `TECHNICAL_OVERVIEW.md`.
* Changing build requirements → update setup instructions in `README.md`.