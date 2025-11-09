import java.util.ArrayList;
import java.util.List;

public class Main {
    public static void main(String[] args) {
        List<byte[]> blocks = new ArrayList<>();
        int size = 1024 * 1024;
        while (true) {
            blocks.add(new byte[size]);
        }
    }
}
