package com.example.util;

public class ReceiptFormatter {
    public String padLeft(String s, int width) {
        if (s == null) {
            return "";
        }
        if (s.length() >= width) {
            return s;
        }
        StringBuilder b = new StringBuilder();
        for (int i = s.length(); i < width; i++) {
            b.append(' ');
        }
        return b.append(s).toString();
    }

    public String truncate(String s, int max) {
        if (s.length() <= max) {
            return s;
        }
        return s.substring(0, max - 3) + "...";
    }
}
