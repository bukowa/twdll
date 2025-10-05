import sys
import re
from pathlib import Path

def get_changelog_for_version(version, changelog_text):
    """
    Parses CHANGELOG.md and extracts changes for a specific version.
    THIS IS AN EXACT COPY of the function from the GitHub Actions script.
    """
    pattern = re.compile(f"## \\[{version}\\](.*?)(?=\\n## \\[|$)", re.DOTALL)
    match = pattern.search(changelog_text)
    if not match:
        # Zwracamy pusty string i kod błędu, aby skrypty hooków mogły to wykryć
        return None

    # Prosta konwersja markdown na HTML (również kopia 1-do-1)
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

def main():
    """Main function to run the validation."""
    if len(sys.argv) < 2:
        print("❌ Błąd: Podaj numer wersji jako argument.")
        print("   Przykład: python .github/scripts/validate_changelog.py 1.2.0")
        sys.exit(1)

    version_to_check = sys.argv[1]
    changelog_file = Path("CHANGELOG.md")

    if not changelog_file.exists():
        print(f"❌ Błąd: Plik {changelog_file} nie został znaleziony w głównym katalogu.")
        sys.exit(1)

    print(f"🔬 Walidacja pliku CHANGELOG.md dla wersji: {version_to_check}\n")

    changelog_text = changelog_file.read_text()
    parsed_html = get_changelog_for_version(version_to_check, changelog_text)

    if parsed_html is None:
        print("❗ Wynik: Nie znaleziono wpisu dla tej wersji w CHANGELOG.md.")
        print("   Upewnij się, że nagłówek ma format: ## [1.2.0]")
        sys.exit(1)
    else:
        print("✅ Sukces! Wygenerowana zawartość, która pojawi się na stronie:")
        print("-" * 40)
        print(parsed_html)
        print("-" * 40)
        sys.exit(0)

if __name__ == "__main__":
    main()