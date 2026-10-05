# Releasing Syncscope

Releases are built by `.github/workflows/release.yml` when a tag matching `v*`
is pushed. The workflow:

1. Builds with Wails on three runners:
   - `darwin/universal` (macos-latest) → `Syncscope_<tag>_darwin_universal.zip` (zipped `.app`)
   - `windows/amd64` (windows-latest) → `Syncscope_<tag>_windows_amd64_installer.exe` (NSIS) and `Syncscope_<tag>_windows_amd64.zip` (portable `.exe`)
   - `linux/amd64` (ubuntu-24.04, `-tags webkit2_41`) → `Syncscope_<tag>_linux_amd64.tar.gz`
2. Stamps the version: `-ldflags "-X main.version=<tag>"` and `info.productVersion`
   in `wails.json` (used for `Info.plist` and the Windows resources; the
   pre-release suffix is dropped there).
3. On macOS, **if the signing secrets exist**, signs the app with a Developer ID
   certificate (hardened runtime), notarizes it with `notarytool` and staples
   the ticket. Without the secrets the macOS build is unsigned.
4. Generates `SHA256SUMS` over all artifacts.
5. Creates the GitHub Release (`softprops/action-gh-release`) with
   auto-generated release notes. Tags containing `-` (e.g. `v0.3.0-rc.1`) are
   marked as pre-releases, which the in-app update check ignores.

Asset names contain `darwin` / `windows` / `linux` and the architecture so the
updater (`internal/updater`) can pick the right download. Keep that scheme if
you change the packaging.

## Cutting a release

```bash
git checkout main && git pull
# make sure CI is green on main, update README / changelog if needed
git tag -a v0.2.0 -m "Syncscope v0.2.0"
git push origin v0.2.0
```

Then watch the *Release* workflow in the Actions tab. When it finishes, edit
the generated notes on the release page if needed.

To rebuild an existing tag (e.g. after fixing the workflow), run *Release* via
**Run workflow** and enter the tag. `softprops/action-gh-release` updates the
existing release and replaces assets with the same name.

Version scheme: [SemVer](https://semver.org) with a `v` prefix. Local builds
report `dev`, which never triggers an update notification.

## Secrets for macOS signing and notarization

Add these under *Settings → Secrets and variables → Actions → Repository
secrets*. Signing runs when the first two are set; notarization needs all five.

| Secret | Content |
|--------|---------|
| `APPLE_CERT_P12_BASE64` | Developer ID Application certificate **and private key**, exported as `.p12`, base64-encoded (`base64 -i cert.p12 \| pbcopy`) |
| `APPLE_CERT_PASSWORD` | Password chosen when exporting the `.p12` |
| `APPLE_ID` | Apple ID e-mail of a member of the developer team |
| `APPLE_TEAM_ID` | 10-character Team ID (developer.apple.com → Membership details) |
| `APPLE_APP_PASSWORD` | App-specific password for that Apple ID (account.apple.com → Sign-In and Security → App-Specific Passwords) |

The workflow imports the certificate into a temporary keychain, signs with
`codesign --force --deep --timestamp --options runtime`, submits a zip with
`xcrun notarytool submit --wait`, then runs `xcrun stapler staple` and
`spctl --assess` before packaging. The keychain is deleted at the end of the job.

### Getting a Developer ID Application certificate

1. Enroll in the [Apple Developer Program](https://developer.apple.com/programs/)
   (paid, individual or organization). Only the *Account Holder* can create
   Developer ID certificates.
2. On a Mac, open **Keychain Access → Certificate Assistant → Request a
   Certificate From a Certificate Authority…**, enter your e-mail, choose
   *Saved to disk*. This creates a `.certSigningRequest` and a private key in
   your login keychain.
3. Go to [Certificates, IDs & Profiles](https://developer.apple.com/account/resources/certificates/list),
   click **+**, choose **Developer ID Application** (G2 Sub-CA), upload the CSR
   and download the `.cer`.
4. Double-click the `.cer` to install it. In Keychain Access → *My
   Certificates*, expand "Developer ID Application: Your Name (TEAMID)" to make
   sure the private key is attached.
5. Right-click the certificate → **Export…** → `.p12`, set a strong password.
   That file and password become `APPLE_CERT_P12_BASE64` / `APPLE_CERT_PASSWORD`.
6. Check locally: `security find-identity -v -p codesigning` should list it.

Alternatively, Xcode → Settings → Accounts → Manage Certificates → **+** →
*Developer ID Application* creates and installs the certificate in one step.

Keep the `.p12` out of the repository and delete local copies after storing
the secret. Developer ID certificates are valid for 5 years; apps signed and
notarized with a timestamp keep working after expiry, but new releases need a
renewed certificate.

### Troubleshooting notarization

- Get the log of a rejected submission:
  `xcrun notarytool log <submission-id> --apple-id … --team-id … --password …`
- "The binary is not signed with a valid Developer ID certificate": the `.p12`
  contains an *Apple Development* or *Mac App Distribution* certificate instead
  of *Developer ID Application*.
- "The signature does not include a secure timestamp" / "hardened runtime not
  enabled": a nested binary was not re-signed; the workflow signs with `--deep`.

## Windows and Linux

Windows builds are not code-signed yet (SmartScreen will warn on first run).
The NSIS installer comes from `build/windows/installer/project.nsi`. Linux
builds need GTK 3 and WebKitGTK 4.1 at runtime
(`sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0`).
