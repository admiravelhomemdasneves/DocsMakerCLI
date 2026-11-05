package com.example.animal;

/**
 * Base type for anything in the sample zoo that can be greeted.
 */
public abstract class Animal {
    protected final String name;

    public Animal(String name) {
        this.name = name;
    }

    public String getName() {
        return name;
    }

    public abstract String sound();
}
