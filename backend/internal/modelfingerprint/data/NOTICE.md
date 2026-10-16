# Public fingerprint data attribution

`library.json` is a reduced, normalized derivative assembled on 2026-10-16. Each
profile retains the exact source URL, pinned Git commit and SHA-256 of the full
original JSON. The runtime does not download or execute upstream programs.

- **KBF**, Ooo0ption and contributors, Apache-2.0, commit
  `68aefbbab879469152aaac9fd988a86c30ee4739`. Retained the ten selected models'
  September 2026 question values, self-test counts, dates and provider labels.
  Prompts, domain bounds/tolerances and the statistical method are adapted from
  `scripts/domains.py` and `scripts/kbf_common.py`. The Go parser is stricter;
  retries, provider pinning and automatic thinking-strategy discovery are not
  copied. See `LICENSE-KBF.txt`.
- **modelprint**, unclecode and contributors, MIT, commit
  `87b529d9c739b2703a2724788609ac26553cb10e`. Retained the latest recorded version's
  numeric tokenizer deltas only, excluding fields marked unavailable. Exact
  prompt text comes from `probes/tok-{english,chinese,code,emoji}.js`. Gemini
  3.7 Flash has no usable tokenizer values; its source is retained solely to
  explain the absence. See `LICENSE-modelprint.txt`.
- **Single-token output distributions as behavioral fingerprints of large
  language models**, Tomáš Bruckner, Prague University of Economics and Business,
  2026, DOI https://doi.org/10.5281/zenodo.21278557, CC-BY-4.0.
  The data here are taken from dreamor's **llm-fingerprint** derivative,
  `c46eacb03a3840f10d94ba0a1cb7ca83e6e54f7f/data/reference.json` (the original
  Zenodo ZIP has not been imported). Retained eight numeric task/language cells
  each for Claude Sonnet 5, Qwen3.7 Max and Qwen3.7 Plus, with original rounded
  probabilities and valid counts. Prompts are the derivative's reconstructed
  `lib/tasks.js` prompts; numeric normalization is stricter. This subset has no
  calibrated identity threshold and is used only for descriptive distances.
  Tool text is MIT (`LICENSE-llm-fingerprint.txt`); data license is
  `LICENSE-CC-BY-4.0.txt`, also https://creativecommons.org/licenses/by/4.0/.

Changes: remove unrelated models, raw generation history and response headers;
normalize field names; preserve zero reference answers; select explicitly
registered full and bare model IDs; include exact prompt texts and domain
metadata. Source/provider labels are the collectors' claims, not independent
verification or endorsement by their authors or model vendors. Full methodology,
coverage and limitations: `docs/public-model-fingerprints.md` at repository root.
