COSMIC sizing prompt for LLM agents

Follow this checklist to size a feature in CFP.

1) Clarify inputs

Ask for: purpose, measurement scope and layer(s), functional users, boundary, persistent storage, required granularity, triggering events, objects of interest and data groups, external interfaces, and error/confirmation messaging. If unknown, state assumptions.

2) Fix granularity

Measure at the functional process level of granularity.

3) Map the work

Identify functional users and a simple context. Derive functional processes from triggering events. One process has one triggering Entry.

4) Model data

List objects of interest. Group attributes into data groups. Rule: different frequency or different key(s) ⇒ different data groups.

5) Identify data movements per process

Use COSMIC types: Entry (E), Exit (X), Read (R), Write (W). Each distinct movement = 1 CFP. Ignore UI control commands. Count one Exit for all error/confirmation messages per process. A process has ≥1 Entry and either an Exit or a Write.

6) Aggregate sizes

CFP(process) = Σ(E)+Σ(X)+Σ(R)+Σ(W). Sum processes only within the defined scope and at comparable decomposition and granularity.

7) Changes

For enhancements, count added, modified, and deleted data movements; then aggregate per rules.

8) Output format

Produce a table and a short note of assumptions and open questions.

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| <name>             | # | # | # | # | sum |
| ...                |   |   |   |   |     |
| TOTAL              |   |   |   |   | N   |

9) Pseudocode aid (use when text is unclear)

If the future code is unclear, draft minimal pseudocode that makes Entries/Exits/Reads/Writes explicit, then count from it. COSMIC permits sizing from available artefacts and design models.

```
process <name>:
  trigger: Entry(<data group from <functional user>>)
  reads:   Read(<data group[s] from persistent storage>)
  writes:  Write(<data group[s] to persistent storage>)
  outputs: Exit(<data group[s] to <functional user>>)
  outputs: Exit(errors)   # one Exit covers all error/confirmation messages
```
