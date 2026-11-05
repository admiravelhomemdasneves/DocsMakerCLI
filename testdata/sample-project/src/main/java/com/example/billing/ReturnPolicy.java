package com.example.billing;

public class ReturnPolicy {
    public boolean canReturn(int daysSincePurchase, String category, boolean opened) {
        if ("PERISHABLE".equals(category)) {
            return false;
        }
        if ("ELECTRONICS".equals(category)) {
            return !opened && daysSincePurchase <= 30;
        }
        return daysSincePurchase <= 14;
    }
}
