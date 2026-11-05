package com.example.billing;

public class TaxIdValidator {
    public boolean isValidNif(String nif) {
        if (nif == null || nif.length() != 9) {
            return false;
        }
        for (int i = 0; i < 9; i++) {
            if (!Character.isDigit(nif.charAt(i))) {
                return false;
            }
        }
        if ("0478".indexOf(nif.charAt(0)) >= 0) {
            return false;
        }
        int sum = 0;
        for (int i = 0; i < 8; i++) {
            sum += (nif.charAt(i) - '0') * (9 - i);
        }
        int check = 11 - (sum % 11);
        if (check >= 10) {
            check = 0;
        }
        return check == nif.charAt(8) - '0';
    }
}
