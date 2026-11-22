# Security policy

kmd treats every Markdown document as untrusted input. A document should
never be able to:

- send control or escape sequences to the terminal (beyond the styling kmd
  itself emits),
- read local files other than the images it references, or display
  anything that is not a valid PNG/JPEG/GIF image,
- make kmd access the network,
- make kmd consume unbounded CPU time or memory,
- run script in the page produced by `--html` (without `--unsafe-html`).

If you find a way to do any of these, it is a vulnerability.

## Supported versions

Security fixes are released for the latest version only.

## Reporting a vulnerability

Please report privately through GitHub:
**[Report a vulnerability](https://github.com/DipaolaFederico/kmd/security/advisories/new)**
(Security tab → "Report a vulnerability"). Include the input that triggers
the problem, the command line, your terminal and the kmd version.

Do not open a public issue or pull request for vulnerabilities. You will get
an acknowledgement within a few days; once a fix is released the advisory
is published and you are credited unless you prefer otherwise.
