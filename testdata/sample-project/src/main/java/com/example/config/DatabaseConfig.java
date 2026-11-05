package com.example.config;

/**
 * NOTE for DocsMakerCli reviewers: the connection string below is a
 * deliberately fake, synthetic credential planted for testing the secret
 * detection gates (Semana 7 do plano) — never a real one.
 */
public class DatabaseConfig {
    private final String connectionString = "jdbc:mysql://admin:hunter2@localhost:3306/sampledb";

    public String connectionString() {
        return connectionString;
    }
}
