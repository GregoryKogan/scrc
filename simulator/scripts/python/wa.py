import sys

def main() -> None:
    data = sys.stdin.read().strip()
    if not data:
        print("1")
        return
    total = sum(int(part) for part in data.split())
    print(total + 1)


if __name__ == "__main__":
    main()
