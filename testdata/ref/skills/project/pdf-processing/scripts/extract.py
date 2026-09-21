"""Extract the text of a PDF."""
import sys

def main(path: str) -> int:
    print(f"extracting {path}")
    return 0

if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1]))
