package com.example.billing;

public class CreditPolicy {
    private static final double NEW_CUSTOMER_CAP = 500.0;

    public boolean canExtendCredit(Customer c, double amount) {
        if (c.getDaysOverdue() > 90) {
            return false;
        }
        double limit = c.getCreditLimit();
        if (c.getMonthsAsCustomer() < 6 && limit > NEW_CUSTOMER_CAP) {
            limit = NEW_CUSTOMER_CAP;
        }
        return c.getBalance() + amount <= limit;
    }
}
