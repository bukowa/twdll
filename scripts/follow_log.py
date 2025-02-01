import time
import threading
import os
import sys


def follow_file(file_path, stop_event):
    try:
        with open(file_path, 'r') as file:
            file.seek(0, 2)  # Move to the end of the file
            print(f"Started monitoring: {file_path}")
            while not stop_event.is_set():
                line = file.readline()
                if line:
                    print(f"[{file_path}] {line}", end='')
                else:
                    time.sleep(0.1)  # Sleep briefly to avoid busy-waiting
    except FileNotFoundError:
        print(f"File not found: {file_path}")
    except PermissionError:
        print(f"Permission denied: {file_path}")
    except Exception as e:
        print(f"Error monitoring {file_path}: {e}")


def follow_files_simultaneously(file_paths):
    threads = []
    stop_event = threading.Event()

    for path in file_paths:
        t = threading.Thread(target=follow_file, args=(path, stop_event))
        t.daemon = True  # Allows threads to exit when the main program exits
        threads.append(t)
        t.start()

    try:
        while True:
            time.sleep(1)  # Keep the main thread alive
    except KeyboardInterrupt:
        print("\nStopping all monitoring...")
        stop_event.set()  # Signal threads to stop
        for t in threads:
            t.join()
        print("All monitoring stopped.")


if __name__ == "__main__":
    # Read file paths from command-line arguments if provided
    if len(sys.argv) > 1:
        file_paths = sys.argv[1:]
    else:
        file_paths = [
            r"C:\Games\Total War - Rome 2\investigate.txt",
        ]

    # Normalize paths for cross-platform compatibility
    file_paths = [os.path.normpath(path) for path in file_paths]

    follow_files_simultaneously(file_paths)
