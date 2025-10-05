
## CA - Builtin Functions

### CliExecute

The `CliExecute` function is used to execute commands with specific flags.

#### Usage:
```lua
CliExecute(flag <args>)
```

#### Categories and Flags:

- **battle** (Category): Execute commands related to battle mode.

    - ***frame_rate_test_fps <number>*** (Flag):

        - **CA Description**: Play at predictable frame rate.
        - **Description**: Modifies the number of frames per second the engine produces during battle and campaign.
        - **Effect**: A higher value results in slower battle units.
        - **Note**: Around `1000`, the pause/forward button may stop working. Higher FPS values can affect gameplay speed, and excessively high values can cause issues like stuttering or freezing.
        - **Example**:

        ```lua
        CliExecute(frame_rate_test_fps", 500)
        ```
    
- ~~toggle_unit_lanterns~~
- terrain_write_html
- report_rigid_models
