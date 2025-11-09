import sys

def main() -> None:
    data = sys.stdin.read().strip()
    if not data:
        print("0")
        return
    total = sum(int(part) for part in data.split())
    print(total)


if __name__ == "__main__":
    main()
