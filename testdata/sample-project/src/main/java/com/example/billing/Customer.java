package com.example.billing;

public class Customer {
    private final String name;
    private final double balance;
    private final double creditLimit;
    private final int daysOverdue;
    private final int monthsAsCustomer;

    public Customer(String name, double balance, double creditLimit, int daysOverdue, int monthsAsCustomer) {
        this.name = name;
        this.balance = balance;
        this.creditLimit = creditLimit;
        this.daysOverdue = daysOverdue;
        this.monthsAsCustomer = monthsAsCustomer;
    }

    public String getName() {
        return name;
    }

    public double getBalance() {
        return balance;
    }

    public double getCreditLimit() {
        return creditLimit;
    }

    public int getDaysOverdue() {
        return daysOverdue;
    }

    public int getMonthsAsCustomer() {
        return monthsAsCustomer;
    }
}
