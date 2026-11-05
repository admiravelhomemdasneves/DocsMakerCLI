package com.example.animal;

import com.example.greeting.Greeter;

public class Dog extends Animal implements Greeter {
    private final Greeter delegate;

    public Dog(String name, Greeter delegate) {
        super(name);
        this.delegate = delegate;
    }

    @Override
    public String sound() {
        return "Woof";
    }

    /**
     * Greets using the configured delegate, falling back to this dog's own name.
     */
    @Override
    public String greet(String who) {
        String own = getName();
        return delegate.greet(own) + " (" + who + ")";
    }
}
