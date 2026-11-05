package com.example.animal;

import com.example.greeting.Greeter;

public class Cat extends Animal implements Greeter {

    public Cat(String name) {
        super(name);
    }

    @Override
    public String sound() {
        return "Meow";
    }

    @Override
    public String greet(String who) {
        return getName() + " ignores " + who;
    }
}
