# Unity Multiplayer Soccer - TODO List

- Use pod affinity to keep all the game instances toghether
- Autoscaling (Down)
    - If removing a node will still keep you above buffer - then cordon the node with the least pods on it
    - (up) if autoscaling up, check to see if there are cordoned nodes to turn back on
    - once a cordoned node is empty, then delete the instance
    - Write minimum and maximum values for scaling
- Make it so a player can't go into the goal area
- Show "GOAL" when a goal happens
- Put a highlight on the goal you are aiming for, so you can tell which is which
- Deal with disconnections/reconnections
- Animate paddle on kick    