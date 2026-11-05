package com.example.billing;

public class TaxCalculator {
    public double vatRate(String region, String category) {
        boolean reduced = "FOOD".equals(category) || "BOOKS".equals(category);
        boolean intermediate = "RESTAURANT".equals(category);
        if ("AZORES".equals(region)) {
            return reduced ? 0.04 : intermediate ? 0.09 : 0.16;
        }
        if ("MADEIRA".equals(region)) {
            return reduced ? 0.05 : intermediate ? 0.12 : 0.22;
        }
        return reduced ? 0.06 : intermediate ? 0.13 : 0.23;
    }
}
