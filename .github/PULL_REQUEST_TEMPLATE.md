<!-- One or two sentences: what changed and why. -->

Closes #<issue>.

Checklist (tick what applies):

- [ ] I added or updated tests covering the changed behaviour
- [ ] I added a .phpt fixture under tests/fixtures/ for changed language or runtime behaviour
- [ ] I updated the documentation under docs/ for this change
- [ ] I ran the default atkins pipeline and it passes end to end
- [ ] I committed the generated files under docs/reference/, and none of the gitignored ones `atkins gen` writes
- [ ] I changed go.mod/sum (third party PRs get auto-rejected on this)
