# QR Screen Relay

Local helper for **authorized** QR login/session relay exercises.

Preferred: download `fishing-qr-relay.zip` from **Workbench → QR Phishing** in the platform UI (includes `config.yaml` template).

1. Create a relay in **Workbench → QR Phishing** and copy the **upload token** (shown once).
2. Put the token into `config.yaml` (`upload_token`).
3. Install deps and run:

```bash
# macOS QR decoder native lib
brew install zbar

python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

# Optional: pick a screen region (ROI) with a drag overlay
python qr_relay.py --pick-region -c config.yaml

python qr_relay.py -c config.yaml
```

The script:

- Uploads **image files only** to `POST /api/qr-relay/frames/`
- Sends `POST /api/qr-relay/heartbeat/` when no new QR is uploaded (keeps console health online)
- Re-decodes QR text on every capture and includes it as multipart field `payload`

Header: `X-QR-Relay-Token: <token>`  
Multipart field: `image` (png/jpeg/gif/webp, ≤ 2 MiB)

Public hosted URL (always latest frame):

`GET /q/{slug}.png`
