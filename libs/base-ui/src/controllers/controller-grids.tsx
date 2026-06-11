import { StatusGrid } from "../components/status-grid";
import type { Status } from "../types";
import { calculateControllerMetrics } from "../utils/controller-calculations";

export const ControllerGrids = ({
  controllers,
}: Pick<Status, "controllers">) => {
  const { totalControllers } = calculateControllerMetrics(controllers);

  return (
    <div className="mb-4">
      <StatusGrid
        title="Controllers"
        accent="cyan"
        headers={["Total"]}
        values={[totalControllers]}
      />
    </div>
  );
};
