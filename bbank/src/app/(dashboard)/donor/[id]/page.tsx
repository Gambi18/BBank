import React from 'react'

const patientDetails = async ({ params }: { params: { id: string } }) => {
    const { id } = await params;

    return (
        <div>details for donor #{id}</div>
    )
}

export default patientDetails