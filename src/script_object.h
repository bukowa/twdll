#pragma once

#include <cstddef>

// A template for reading from memory
template <typename T>
T read_from(void* base, size_t offset) {
    return *reinterpret_cast<T*>(static_cast<char*>(base) + offset);
}

// A template for writing to memory
template <typename T>
void write_to(void* base, size_t offset, T value) {
    *reinterpret_cast<T*>(static_cast<char*>(base) + offset) = value;
}

class ScriptObject {
protected:
    void* object_ptr;
    size_t real_pointer_offset;

public:
    ScriptObject(void* ptr, size_t offset) : object_ptr(ptr), real_pointer_offset(offset) {}

    void* get_real_object() const {
        if (!object_ptr) {
            return nullptr;
        }
        return read_from<void*>(object_ptr, real_pointer_offset);
    }

    template <typename T>
    T get_value(size_t offset) const {
        void* real_obj = get_real_object();
        if (!real_obj) {
            return T{}; // Return default value if object is null
        }
        return read_from<T>(real_obj, offset);
    }

    template <typename T>
    void set_value(size_t offset, T value) {
        void* real_obj = get_real_object();
        if (real_obj) {
            write_to<T>(real_obj, offset, value);
        }
    }
};
