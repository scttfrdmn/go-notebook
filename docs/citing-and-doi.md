# Citing go-notebook, and minting a DOI

This project ships a [`CITATION.cff`](../CITATION.cff) (GitHub renders a **"Cite this repository"** button from it) and a [`codemeta.json`](../codemeta.json). Neither mints a DOI on its own — a DOI comes from an archiving service. Below are the options and the exact manual steps, because the archiving hooks require web authentication that can't be scripted from the repo.

## Option A — Zenodo (recommended for the software)

Zenodo archives each GitHub release and issues two DOIs: a **versioned DOI** (this exact release) and a **concept DOI** (always resolves to the latest version). This is the standard for research software.

One-time setup:

1. Sign in at <https://zenodo.org> with your GitHub account.
2. Go to <https://zenodo.org/account/settings/github/> and flip the toggle **on** for `scttfrdmn/go-notebook`.
3. Cut a GitHub release (a tag like `v0.2.0`, published — not just a draft). Zenodo captures it automatically.
4. Back on the Zenodo GitHub page, the repo now shows a DOI badge. Copy the **concept DOI** (the "all versions" one).
5. Paste it into `CITATION.cff` under the commented `doi:` line, uncomment it, and commit. GitHub's cite button and downstream tools will then surface the DOI.

Zenodo reads `CITATION.cff` / `codemeta.json` to populate the record's authors, license, and abstract, so those are already prepared.

## Option B — arXiv (for the paper)

`docs/paper.md` is written as a system paper. To make it a citable **preprint** with an arXiv ID (arXiv also issues DOIs):

1. Convert `paper.md` to LaTeX or a PDF (pandoc: `pandoc docs/paper.md -o paper.pdf`, then refine — arXiv wants LaTeX source ideally).
2. Submit at <https://arxiv.org/submit>; category `cs.SE` (Software Engineering) or `cs.PL` (Programming Languages) fits.
3. Add the resulting arXiv ID / DOI to `CITATION.cff` as a `preferred-citation` entry.

This is more effort than Zenodo (it needs the paper as a formatted document) and is worth it if you want the paper discoverable in academic search, not just a citable software handle.

## Option C — Software Heritage (preservation)

<https://archive.softwareheritage.org> archives the repository permanently and issues a `swh:` identifier (a stable citable reference, though not a DOI). It's automatic and complements Zenodo — worth submitting the repo URL once for long-term preservation.

## Option D — a reviewed venue (JOSS)

The [Journal of Open Source Software](https://joss.theoj.org) gives a peer-reviewed publication + DOI for research software, if you want academic credit rather than only a citable handle. It's a review process (weeks), not a one-click step.

## Recommendation

**Zenodo + `CITATION.cff`** is the baseline: cheap, standard, and it gives the cite button now and a DOI on the next release. Add **arXiv** if you want the paper itself discoverable. The two are complementary.
