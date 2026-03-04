# AllureDeck — GitHub Pages

This branch (`gh-pages`) contains the static landing page for the AllureDeck Helm chart, served at [alluredeck.kutlak.cc](https://alluredeck.kutlak.cc).

The page provides quick-start installation instructions for the OCI-published Helm chart and an overview of key features.

## Structure

- `index.html` — Self-contained landing page (inline CSS, no build step)
- `.nojekyll` — Bypasses Jekyll processing on GitHub Pages
- `CNAME` — Custom domain configuration

## Development

The Helm chart source and application code live on the [`main`](https://github.com/mkutlak/alluredeck/tree/main) branch.

To preview locally:

```sh
python3 -m http.server 8080
# open http://localhost:8080
```
