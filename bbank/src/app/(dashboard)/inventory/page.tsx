import AreaPlaceholder from '@/components/AreaPlaceholder'

export default function InventoryArea() {
    return (
        <AreaPlaceholder
            area="Inventory"
            workItem="WI-45"
            does={[
                'Track every unit through its status changes',
                'Process whole blood into components',
                'Move, expire and discard stock with a full audit trail',
            ]}
        />
    )
}
