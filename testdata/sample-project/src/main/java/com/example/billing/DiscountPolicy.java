package com.example.billing;

public class DiscountPolicy {
    public double discountFor(int quantity, double orderTotal) {
        if (orderTotal < 20.0) {
            return 0.0;
        }
        if (quantity >= 100) {
            return 0.15;
        } else if (quantity >= 50) {
            return 0.10;
        } else if (quantity >= 10) {
            return 0.05;
        }
        return 0.0;
    }
}
