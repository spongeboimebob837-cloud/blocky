# Colab / Kaggle Deployment Packaging (Track C, C2)

The proposal lists "Colab/Kaggle deployment" as one of the four implementation
areas. Because the Fabric *network* runs on the local machine (Docker), the
Colab/Kaggle notebooks only need the **Python bridge**, not a live peer.

## What goes into the notebook runtime

1. `app/src/blockchain.py` — the `FabricBridge` / `Prediction` module
   (pure Python stdlib; already validated above with `python3 -c`).
2. A mounted/bundled copy of the `peer` CLI + the org MSP material if the
   notebook itself must submit. Otherwise the notebook exports predictions to
   JSONL and the local machine anchors them (recommended for PoC).
3. `requirements.txt` — no additional dependency is required for the bridge
   (`hashlib`, `json`, `subprocess`, `dataclasses` are stdlib).

## Two supported modes

### Mode A — notebook only predicts (recommended, PoC)
The fine-tuning/eval notebook writes `[{row_id, text, label, confidence}]` rows
to `data/out/predictions_nso.jsonl`. A local step anchors them:

```bash
python3 -m src.blockchain --infile data/out/predictions_nso.jsonl \
  --language nso --model-version afroxlmr-large-nso-v1.0
```

### Mode B — notebook submits directly
Requires Fabric binaries + crypto bundled (or SSH tunnel to the test network).
```python
from src.blockchain import FabricBridge, submit_pipeline_output
bridge = FabricBridge(org="org1")
submit_pipeline_output(rows, language="nso", model_version="afroxlmr-large-nso-v1.0", bridge=bridge)
```

## Checklist (aligns with proposal section 4)
- [x] Pipeline produces `row_id, text, label, confidence` (data_prep.py)
- [x] Bridge hashes text with sha256 and never sends raw text (DATA_MODEL.md §1.a)
- [x] Bridge is stdlib-only so Colab/Kaggle needs no extra `pip install`
- [x] Immutability + provenance enforced chaincode-side (MSP from tx context)
