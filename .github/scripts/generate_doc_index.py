import os
import sys
import re
from pathlib import Path

def get_changelog_for_version(version, changelog_text):
    """Parses CHANGELOG.md and extracts changes for a specific version."""
    pattern = re.compile(f"## \\[{version}\\](.*?)(?=\\n## \\[|$)", re.DOTALL)
    match = pattern.search(changelog_text)
    if not match:
        return "<p>No changelog entry found for this version.</p>"

    content = match.group(1).strip()
    html_content = "<ul>\n"
    for line in content.splitlines():
        line = line.strip()
        if line.startswith(('-', '*')):
            html_content += f"  <li>{line[1:].strip()}</li>\n"
        elif line.startswith(('###', '##')):
            html_content += f"</ul><h4>{line.lstrip('#').strip()}</h4><ul>\n"
    html_content += "</ul>"
    return html_content

def generate_version_list(versions, repo_slug, changelog_text, repo_path):
    """Generates HTML list items for each version with its changelog."""
    html = ""
    sorted_versions = sorted([v for v in versions if v != 'nightly'], reverse=True)
    if 'nightly' in versions:
        sorted_versions.insert(0, 'nightly')

    for version in sorted_versions:
        html += "<li>"
        html += f'<div class="version-header"><a href="./{version}/" class="version-link">{version}</a>'
        if version == 'nightly':
            release_url = f"https://github.com/{repo_slug}/releases/tag/nightly"
            html += f'<a href="{release_url}" class="release-link" target="_blank">Download Nightly Build</a>'
        else:
            release_tag = f"v{version}"
            release_url = f"https://github.com/{repo_slug}/releases/tag/{release_tag}"
            html += f'<a href="{release_url}" class="release-link" target="_blank">Download Release</a>'
        html += "</div>"

        # ===============================================================
        # KLUCZOWA ZMIANA JEST TUTAJ
        # Dodajemy changelog TYLKO jeśli wersja NIE JEST 'nightly'
        # ===============================================================
        if version != 'nightly':
            html += '<details class="changelog-details">'
            html += '<summary>View Changes</summary>'
            # Logika dla oficjalnych wydań pozostaje bez zmian
            html += get_changelog_for_version(version, changelog_text)
            html += "</details>"

        html += "</li>\n"

    return html

def main():
    template_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])
    versions_dir = Path(sys.argv[3])
    new_version = sys.argv[4]
    repo_path = Path(sys.argv[5])
    repo_slug = os.getenv("GITHUB_REPOSITORY")
    if not repo_slug:
        print("Error: GITHUB_REPOSITORY environment variable not set.")
        sys.exit(1)
    changelog_path = repo_path / "CHANGELOG.md"
    changelog_text = changelog_path.read_text() if changelog_path.exists() else ""
    existing_versions = {item.name for item in versions_dir.iterdir() if item.is_dir() and not item.name.startswith('.')}
    existing_versions.add(new_version)
    template_content = template_path.read_text()
    version_list_html = generate_version_list(existing_versions, repo_slug, changelog_text, repo_path)
    final_html = template_content.replace("<!-- VERSION_LIST_PLACEHOLDER -->", version_list_html)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(final_html)
    print(f"Successfully generated index.html at {output_path}")

if __name__ == "__main__":
    main()