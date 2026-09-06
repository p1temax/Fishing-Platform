# Placeholder guide

Write the markers below into HTML or email bodies. The platform replaces them with real values when a project is deployed or an email is sent.

The two sets are **not interchangeable**:

- Do **not** put **project landing-page** markers in email
- Do **not** put **email** markers in project HTML

Page Library / Page Builder preview does **not** replace project placeholders. Upload the HTML to **Projects**, then build/start the project.

---

## 1. Project landing pages (Projects)

Use these placeholders in HTML uploaded to a project.

| Placeholder | Purpose | Guidance |
|-------------|---------|----------|
| `{{SUBMIT_URL}}` | Form submit URL | **Recommended.** Use as the form `action` or API URL |
| `{{REDIRECT_URL}}` | Original site URL after submit | Optional. Comes from the project “Original URL” field |
| `{{QR_RELAY_URL}}` | Live QR image URL | For scan-to-login UIs. Bind a QR relay on the project |
| `{{QR_RELAY_IMG}}` | Full QR `<img>` tag | Same as above; insert this token instead of writing your own `img` |

### Example

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Login</title>
</head>
<body>
  <h1>Sign in</h1>

  <form method="POST" action="{{SUBMIT_URL}}">
    <input name="username" placeholder="Username" required />
    <input name="password" type="password" placeholder="Password" required />
    <button type="submit">Login</button>
  </form>

  <!-- Scan login: create a relay under QR Phishing, then bind it on the project -->
  <h2>Scan to sign in</h2>
  {{QR_RELAY_IMG}}

  <!-- Or write the image tag yourself:
  <img src="{{QR_RELAY_URL}}" alt="Scan to sign in" width="240" height="240" />
  -->

  <p><a href="{{REDIRECT_URL}}">Back to official site</a></p>
</body>
</html>
```

### Steps (when using QR)

1. Workbench → **QR Phishing**: create a relay and run the local capture script  
2. **Projects**: create or edit a project, upload HTML that includes the placeholders, select the relay under “QR relay”  
3. Build and start the project  
4. Open the project URL and confirm the submit URL and QR image render  

### Notes

- After changing the original URL, rebinding the relay, or rotating the public slug, **rebuild or restart** the project.  
- If no relay is bound, or the relay is paused, `{{QR_RELAY_URL}}` / `{{QR_RELAY_IMG}}` become empty.  
- `{{QR_RELAY_IMG}}` is a full tag. Put it on its own line; do not write `src="{{QR_RELAY_IMG}}"`.

---

## 2. Email body (Workbench → Send Email)

Use these placeholders in campaign bodies. Each recipient is rendered separately when the message is sent.

| Placeholder | Purpose | Guidance |
|-------------|---------|----------|
| `{{email}}` | Current recipient address | Use when you need a greeting |
| `{{click_url}}` | Trackable click URL | **Recommended.** Use as button/link `href` |
| `{{landing_url}}` | Landing page URL | Usually unnecessary; prefer `{{click_url}}` |
| `{{open_pixel}}` | Open-tracking pixel URL | Optional; with “Track opens” enabled it can be handled automatically |

### HTML example

```html
<p>Hello, {{email}}:</p>
<p>Please complete verification:</p>
<p><a href="{{click_url}}">Verify now</a></p>
```

### Plain-text example

```text
Hello, {{email}}:
Please open the following link to verify:
{{click_url}}
```

### Notes

- Put `{{click_url}}` on a clickable link; do not rely on a raw landing URL alone.  
- Open tracking applies to **HTML** mail only.  
- Do not mix email placeholders with project landing-page placeholders.
