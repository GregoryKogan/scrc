import java.util.Scanner;

public class Main {
    public static void main(String[] args) {
        Scanner scanner = new Scanner(System.in);
        long total = 0;
        while (scanner.hasNextLong()) {
            total += scanner.nextLong();
        }
        System.out.println(total);
    }
}
